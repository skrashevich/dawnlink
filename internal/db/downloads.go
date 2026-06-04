package db

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

const (
	RouteWorkflow = "workflow"
	RouteRun      = "run"
	RouteArtifact = "artifact"
)

// DownloadRecord captures one artifact ZIP download served by dawnlink.
type DownloadRecord struct {
	RouteKind    string
	Owner        string
	Repo         string
	ArtifactName string
	Workflow     string
	Branch       string
	RunID        int64
	ArtifactID   int64
	StatusFilter string
	RunEvent     string
	PrivateLink  bool
	RequestHost  string
	UserAgent    string
	Referer      string
	ClientIPHash string
	Path         string
}

type DownloadEventRow struct {
	ID           int64
	CreatedAt    time.Time
	RouteKind    string
	Owner        string
	Repo         string
	ArtifactName string
	Workflow     string
	Branch       string
	RunID        int64
	ArtifactID   int64
	StatusFilter string
	RunEvent     string
	PrivateLink  bool
	RequestHost  string
	UserAgent    string
	Referer      string
	Path         string
}

type CountRow struct {
	Label string
	Count int64
}

type DayCount struct {
	Day   string
	Count int64
}

type DownloadAnalyticsSummary struct {
	TotalAll      int64
	Total24h      int64
	Total7d       int64
	Total30d      int64
	PrivateCount  int64
	PublicCount   int64
	UniqueRepos   int64
	UniqueClients int64
	ByDay         []DayCount
	ByHour        []DayCount
	TopRepos      []CountRow
	TopArtifacts  []CountRow
	TopWorkflows  []CountRow
	TopBranches   []CountRow
	TopRouteKinds []CountRow
	TopReferers   []CountRow
	Recent        []DownloadEventRow
}

func (s *Store) initDownloads() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS download_events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  created_at TEXT NOT NULL,
  route_kind TEXT NOT NULL,
  owner TEXT NOT NULL,
  repo TEXT NOT NULL,
  artifact_name TEXT NOT NULL,
  workflow TEXT NOT NULL DEFAULT '',
  branch TEXT NOT NULL DEFAULT '',
  run_id INTEGER NOT NULL DEFAULT 0,
  artifact_id INTEGER NOT NULL DEFAULT 0,
  status_filter TEXT NOT NULL DEFAULT '',
  run_event TEXT NOT NULL DEFAULT '',
  private_link INTEGER NOT NULL DEFAULT 0,
  request_host TEXT NOT NULL DEFAULT '',
  user_agent TEXT NOT NULL DEFAULT '',
  referer TEXT NOT NULL DEFAULT '',
  client_ip_hash TEXT NOT NULL DEFAULT '',
  path TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS download_events_created_at ON download_events(created_at);
CREATE INDEX IF NOT EXISTS download_events_owner_repo ON download_events(owner, repo);
CREATE INDEX IF NOT EXISTS download_events_artifact ON download_events(artifact_name);
`)
	return err
}

func (s *Store) RecordDownload(rec DownloadRecord) error {
	now := time.Now().UTC().Format(time.RFC3339)
	priv := 0
	if rec.PrivateLink {
		priv = 1
	}
	_, err := s.db.Exec(`
INSERT INTO download_events (
  created_at, route_kind, owner, repo, artifact_name, workflow, branch,
  run_id, artifact_id, status_filter, run_event, private_link,
  request_host, user_agent, referer, client_ip_hash, path
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		now, rec.RouteKind, rec.Owner, rec.Repo, rec.ArtifactName, rec.Workflow, rec.Branch,
		rec.RunID, rec.ArtifactID, rec.StatusFilter, rec.RunEvent, priv,
		rec.RequestHost, truncate(rec.UserAgent, 512), truncate(rec.Referer, 512),
		rec.ClientIPHash, truncate(rec.Path, 512))
	return err
}

func (s *Store) PurgeDownloadEventsOlderThan(days int) (int64, error) {
	if days <= 0 {
		return 0, nil
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -days).Format(time.RFC3339)
	res, err := s.db.Exec(`DELETE FROM download_events WHERE created_at < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// DownloadAnalyticsSummary returns aggregated download stats limited to the given repositories.
func (s *Store) DownloadAnalyticsSummary(refs []RepoRef, recentLimit int) (*DownloadAnalyticsSummary, error) {
	if recentLimit <= 0 {
		recentLimit = 50
	}
	if recentLimit > 200 {
		recentLimit = 200
	}
	scope, scopeArgs := repoScopeClause(refs)
	out := &DownloadAnalyticsSummary{}
	now := time.Now().UTC()
	if err := s.scanCount(&out.TotalAll, `SELECT COUNT(*) FROM download_events WHERE 1=1`+scope, scopeArgs...); err != nil {
		return nil, err
	}
	if err := s.scanCount(&out.Total24h, `SELECT COUNT(*) FROM download_events WHERE created_at >= ?`+scope,
		appendScopeArgs(scopeArgs, now.Add(-24*time.Hour).Format(time.RFC3339))...); err != nil {
		return nil, err
	}
	if err := s.scanCount(&out.Total7d, `SELECT COUNT(*) FROM download_events WHERE created_at >= ?`+scope,
		appendScopeArgs(scopeArgs, now.AddDate(0, 0, -7).Format(time.RFC3339))...); err != nil {
		return nil, err
	}
	if err := s.scanCount(&out.Total30d, `SELECT COUNT(*) FROM download_events WHERE created_at >= ?`+scope,
		appendScopeArgs(scopeArgs, now.AddDate(0, 0, -30).Format(time.RFC3339))...); err != nil {
		return nil, err
	}
	if err := s.scanCount(&out.PrivateCount, `SELECT COUNT(*) FROM download_events WHERE private_link = 1`+scope, scopeArgs...); err != nil {
		return nil, err
	}
	out.PublicCount = out.TotalAll - out.PrivateCount
	if err := s.scanCount(&out.UniqueRepos, `SELECT COUNT(DISTINCT owner || '/' || repo) FROM download_events WHERE 1=1`+scope, scopeArgs...); err != nil {
		return nil, err
	}
	if err := s.scanCount(&out.UniqueClients, `SELECT COUNT(DISTINCT client_ip_hash) FROM download_events WHERE client_ip_hash != ''`+scope, scopeArgs...); err != nil {
		return nil, err
	}
	var err error
	out.ByDay, err = s.downloadCountsByExpr(`strftime('%Y-%m-%d', created_at)`, 30, scope, scopeArgs)
	if err != nil {
		return nil, err
	}
	out.ByHour, err = s.downloadCountsByExpr(`strftime('%Y-%m-%d %H:00', created_at)`, 48, scope, scopeArgs)
	if err != nil {
		return nil, err
	}
	out.TopRepos, err = s.downloadTop(`owner || '/' || repo`, 15, scope, scopeArgs)
	if err != nil {
		return nil, err
	}
	out.TopArtifacts, err = s.downloadTop(`owner || '/' || repo || ' → ' || artifact_name`, 20, scope, scopeArgs)
	if err != nil {
		return nil, err
	}
	out.TopWorkflows, err = s.downloadTopFiltered(`workflow`, `workflow != ''`, 15, scope, scopeArgs)
	if err != nil {
		return nil, err
	}
	out.TopBranches, err = s.downloadTopFiltered(`branch`, `branch != ''`, 15, scope, scopeArgs)
	if err != nil {
		return nil, err
	}
	out.TopRouteKinds, err = s.downloadTop(`route_kind`, 5, scope, scopeArgs)
	if err != nil {
		return nil, err
	}
	out.TopReferers, err = s.downloadTopFiltered(`referer`, `referer != ''`, 10, scope, scopeArgs)
	if err != nil {
		return nil, err
	}
	out.Recent, err = s.downloadRecent(recentLimit, scope, scopeArgs)
	return out, err
}

func (s *Store) scanCount(dst *int64, query string, args ...any) error {
	return s.db.QueryRow(query, args...).Scan(dst)
}

func (s *Store) downloadCountsByExpr(expr string, limit int, scope string, scopeArgs []any) ([]DayCount, error) {
	q := fmt.Sprintf(`
SELECT %s AS bucket, COUNT(*) AS c
FROM download_events
WHERE 1=1%s
GROUP BY bucket
ORDER BY bucket DESC
LIMIT ?`, expr, scope)
	args := append(append([]any{}, scopeArgs...), limit)
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var raw []DayCount
	for rows.Next() {
		var d DayCount
		if err := rows.Scan(&d.Day, &d.Count); err != nil {
			return nil, err
		}
		raw = append(raw, d)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i, j := 0, len(raw)-1; i < j; i, j = i+1, j-1 {
		raw[i], raw[j] = raw[j], raw[i]
	}
	return raw, nil
}

func (s *Store) downloadTop(labelExpr string, limit int, scope string, scopeArgs []any) ([]CountRow, error) {
	q := fmt.Sprintf(`
SELECT %s AS label, COUNT(*) AS c
FROM download_events
WHERE 1=1%s
GROUP BY label
ORDER BY c DESC, label ASC
LIMIT ?`, labelExpr, scope)
	args := append(append([]any{}, scopeArgs...), limit)
	return s.queryCountRows(q, args...)
}

func (s *Store) downloadTopFiltered(labelExpr, where string, limit int, scope string, scopeArgs []any) ([]CountRow, error) {
	q := fmt.Sprintf(`
SELECT %s AS label, COUNT(*) AS c
FROM download_events
WHERE %s%s
GROUP BY label
ORDER BY c DESC, label ASC
LIMIT ?`, labelExpr, where, scope)
	args := append(append([]any{}, scopeArgs...), limit)
	return s.queryCountRows(q, args...)
}

func (s *Store) queryCountRows(query string, args ...any) ([]CountRow, error) {
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CountRow
	for rows.Next() {
		var row CountRow
		if err := rows.Scan(&row.Label, &row.Count); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *Store) downloadRecent(limit int, scope string, scopeArgs []any) ([]DownloadEventRow, error) {
	q := `
SELECT id, created_at, route_kind, owner, repo, artifact_name, workflow, branch,
  run_id, artifact_id, status_filter, run_event, private_link,
  request_host, user_agent, referer, path
FROM download_events
WHERE 1=1` + scope + `
ORDER BY id DESC
LIMIT ?`
	args := append(append([]any{}, scopeArgs...), limit)
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DownloadEventRow
	for rows.Next() {
		var row DownloadEventRow
		var createdStr string
		var priv int
		if err := rows.Scan(
			&row.ID, &createdStr, &row.RouteKind, &row.Owner, &row.Repo, &row.ArtifactName,
			&row.Workflow, &row.Branch, &row.RunID, &row.ArtifactID, &row.StatusFilter, &row.RunEvent,
			&priv, &row.RequestHost, &row.UserAgent, &row.Referer, &row.Path,
		); err != nil {
			return nil, err
		}
		row.PrivateLink = priv != 0
		if t, err := time.Parse(time.RFC3339, createdStr); err == nil {
			row.CreatedAt = t
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}

// HashClientIP returns a stable pseudonymous identifier for an IP address.
func HashClientIP(appSecret, ip string) string {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return ""
	}
	h := sha256.New()
	h.Write([]byte(appSecret))
	h.Write([]byte{0})
	h.Write([]byte(ip))
	return hex.EncodeToString(h.Sum(nil))[:16]
}