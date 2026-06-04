package db

import (
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	_ "modernc.org/sqlite"

	"github.com/skrashevich/dawnlink/internal/github"
)

type Store struct {
	db        *sql.DB
	appSecret string
	ghApp     *github.AppAuth
	fallback  int64
}

func Open(path, appSecret string, ghApp *github.AppAuth, fallbackInstallID int64) (*Store, error) {
	d, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	d.SetMaxOpenConns(1)
	s := &Store{db: d, appSecret: appSecret, ghApp: ghApp, fallback: fallbackInstallID}
	if err := s.init(); err != nil {
		_ = d.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) init() error {
	_, err := s.db.Exec(`
PRAGMA busy_timeout = 5000;
CREATE TABLE IF NOT EXISTS installations (
  repo_owner TEXT NOT NULL,
  installation_id INTEGER NOT NULL,
  public_repos TEXT NOT NULL,
  private_repos TEXT NOT NULL,
  UNIQUE(repo_owner)
)`)
	return err
}

type RepoInstallation struct {
	RepoOwner      string
	InstallationID int64
	PublicRepos    DelimitedSet
	PrivateRepos   DelimitedSet
}

type DelimitedSet struct {
	items map[string]struct{}
}

func NewDelimitedSet(raw string) DelimitedSet {
	ds := DelimitedSet{items: make(map[string]struct{})}
	for p := range strings.SplitSeq(raw, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			ds.items[strings.ToLower(p)] = struct{}{}
		}
	}
	return ds
}

func (d DelimitedSet) Contains(name string) bool {
	_, ok := d.items[strings.ToLower(name)]
	return ok
}

func (d DelimitedSet) AnyMatch(fn func(string) bool) bool {
	for k := range d.items {
		if fn(k) {
			return true
		}
	}
	return false
}

func (d DelimitedSet) String() string {
	var parts []string
	for k := range d.items {
		parts = append(parts, k)
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}

func (s *Store) Read(repoOwner string) (*RepoInstallation, error) {
	row := s.db.QueryRow(`
SELECT installation_id, public_repos, private_repos FROM installations
WHERE repo_owner = ? COLLATE NOCASE LIMIT 1`, repoOwner)
	var inst RepoInstallation
	var pub, priv string
	err := row.Scan(&inst.InstallationID, &pub, &priv)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	inst.RepoOwner = repoOwner
	inst.PublicRepos = NewDelimitedSet(pub)
	inst.PrivateRepos = NewDelimitedSet(priv)
	return &inst, nil
}

func (s *Store) Write(inst *RepoInstallation) error {
	_, err := s.db.Exec(`
REPLACE INTO installations (repo_owner, installation_id, public_repos, private_repos)
VALUES (?, ?, ?, ?)`,
		inst.RepoOwner, inst.InstallationID, inst.PublicRepos.String(), inst.PrivateRepos.String())
	return err
}

func (s *Store) Delete(repoOwner string) error {
	_, err := s.db.Exec(`DELETE FROM installations WHERE repo_owner = ?`, repoOwner)
	return err
}

func (s *Store) Password(inst *RepoInstallation, repoName string) string {
	h := hmac.New(sha256.New, []byte(s.appSecret))
	fmt.Fprintf(h, "%d\n%s\n%s", inst.InstallationID, inst.RepoOwner, repoName)
	return hex.EncodeToString(h.Sum(nil))[:40]
}

func (s *Store) Verify(inst *RepoInstallation, repoName, h string) (string, error) {
	if inst.PublicRepos.AnyMatch(func(r string) bool {
		return strings.EqualFold(r, repoName)
	}) {
		return "", nil
	}
	if h != "" && inst.PrivateRepos.Contains(repoName) {
		if hmac.Equal([]byte(s.Password(inst, repoName)), []byte(h)) {
			return h, nil
		}
	}
	return "", fmt.Errorf("repository not found")
}

func (s *Store) VerifiedToken(repoOwner, repoName, h string) (github.Token, string, error) {
	inst, err := s.Read(repoOwner)
	if err != nil {
		return nil, "", err
	}
	if inst != nil {
		_, err := s.Verify(inst, repoName, h)
		if err != nil && inst.PrivateRepos.Contains(repoName) {
			return nil, "", err
		}
		if err == nil {
			tok, err := s.ghApp.InstallationToken(inst.InstallationID)
			if err == nil && tok != nil {
				pw := ""
				if inst.PrivateRepos.Contains(repoName) {
					pw = s.Password(inst, repoName)
				}
				return tok, pw, nil
			}
		}
	}
	if s.fallback != 0 {
		tok, err := s.ghApp.InstallationToken(s.fallback)
		return tok, "", err
	}
	return nil, "", fmt.Errorf("no installation token available")
}

func (s *Store) RefreshFromInstallation(inst github.Installation, userToken github.Token) (*RepoInstallation, error) {
	var pub, priv []string
	var token github.Token
	if userToken != nil {
		token = userToken
	} else {
		t, err := s.ghApp.InstallationToken(inst.ID)
		if err != nil {
			return nil, err
		}
		token = t
	}
	repos, err := github.ListInstallationRepos(token, inst.ID, userToken != nil)
	if err != nil {
		return nil, err
	}
	for _, r := range repos {
		if !strings.EqualFold(r.Owner, inst.Account.Login) {
			continue
		}
		if r.Private {
			priv = append(priv, r.Name)
		} else {
			pub = append(pub, r.Name)
		}
	}
	ri := &RepoInstallation{
		RepoOwner:      inst.Account.Login,
		InstallationID: inst.ID,
		PublicRepos:    NewDelimitedSet(strings.Join(pub, ",")),
		PrivateRepos:   NewDelimitedSet(strings.Join(priv, ",")),
	}
	return ri, s.Write(ri)
}
