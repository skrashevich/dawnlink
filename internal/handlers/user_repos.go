package handlers

import (
	"net/http"
	"sort"
	"strings"
	"sync"

	"github.com/skrashevich/dawnlink/internal/db"
	"github.com/skrashevich/dawnlink/internal/github"
)

func (s *Server) userRepoRefs(userTok github.UserToken) ([]db.RepoRef, error) {
	installs, err := github.ListUserInstallations(userTok)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{})
	var refs []db.RepoRef
	for _, inst := range installs {
		ri, err := s.store.RefreshFromInstallation(inst, userTok)
		if err != nil {
			return nil, err
		}
		for _, name := range ri.PublicRepos.Sorted() {
			key := strings.ToLower(ri.RepoOwner) + "/" + strings.ToLower(name)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			refs = append(refs, db.RepoRef{Owner: ri.RepoOwner, Repo: name})
		}
		for _, name := range ri.PrivateRepos.Sorted() {
			key := strings.ToLower(ri.RepoOwner) + "/" + strings.ToLower(name)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			refs = append(refs, db.RepoRef{Owner: ri.RepoOwner, Repo: name})
		}
	}
	return refs, nil
}

func (s *Server) loadDashboardAccounts(r *http.Request, userTok github.UserToken) ([]dashboardAccount, int, error) {
	installs, err := github.ListUserInstallations(userTok)
	if err != nil {
		return nil, 0, err
	}
	var accounts []dashboardAccount
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error
	var errOnce sync.Once
	for _, inst := range installs {
		wg.Go(func() {
			ri, err := s.store.RefreshFromInstallation(inst, userTok)
			if err != nil {
				errOnce.Do(func() { firstErr = err })
				return
			}
			acc := dashboardAccount{Owner: ri.RepoOwner}
			for _, name := range ri.PublicRepos.Sorted() {
				acc.PublicRepos = append(acc.PublicRepos, dashboardRepo{
					Name: name,
					URL:  s.dashboardRepoURL(r, ri.RepoOwner, name, false, ""),
				})
			}
			for _, name := range ri.PrivateRepos.Sorted() {
				acc.PrivateRepos = append(acc.PrivateRepos, dashboardRepo{
					Name: name,
					URL:  s.dashboardRepoURL(r, ri.RepoOwner, name, true, s.store.Password(ri, name)),
				})
			}
			mu.Lock()
			accounts = append(accounts, acc)
			mu.Unlock()
		})
	}
	wg.Wait()
	if firstErr != nil {
		return nil, 0, firstErr
	}
	sort.Slice(accounts, func(i, j int) bool {
		return strings.ToLower(accounts[i].Owner) < strings.ToLower(accounts[j].Owner)
	})
	totalRepos := 0
	for _, acc := range accounts {
		totalRepos += len(acc.PublicRepos) + len(acc.PrivateRepos)
	}
	return accounts, totalRepos, nil
}
