package handlers

import (
	"fmt"
	"strconv"

	"github.com/skrashevich/dawnlink/internal/github"
)

type indexExampleIDs struct {
	CheckSuiteID int64
	ArtifactID   int64
	RunID        int64
	JobID        int64
}

func (ex workflowExample) cacheKey() string {
	return ex.owner + "/" + ex.repo + "/" + ex.workflow + "/" + ex.branch
}

func (s *Server) cachedIndexExampleIDs(ex workflowExample) (indexExampleIDs, error) {
	return s.indexExamplesCache.Get(ex.cacheKey(), func() (indexExampleIDs, error) {
		token, _, err := s.store.VerifiedToken(ex.owner, ex.repo, "")
		if err != nil {
			return indexExampleIDs{}, err
		}
		run, err := s.getLatestRun(ex.owner, ex.repo, s.normalizeWorkflow(ex.workflow), ex.branch, "success", token)
		if err != nil {
			return indexExampleIDs{}, err
		}
		arts, err := github.ListRunArtifacts(ex.owner, ex.repo, run.ID, token)
		if err != nil {
			return indexExampleIDs{}, err
		}
		if len(arts) == 0 {
			return indexExampleIDs{}, fmt.Errorf("no artifacts for index examples")
		}
		art := pickExampleArtifact(arts, ex.artifact)
		jobs, err := github.ListWorkflowRunJobs(ex.owner, ex.repo, run.ID, token)
		if err != nil {
			return indexExampleIDs{}, err
		}
		if len(jobs) == 0 {
			return indexExampleIDs{}, fmt.Errorf("no jobs for index examples")
		}
		suiteID := run.CheckSuiteID()
		if suiteID == 0 {
			return indexExampleIDs{}, fmt.Errorf("workflow run has no check suite id")
		}
		return indexExampleIDs{
			CheckSuiteID: suiteID,
			ArtifactID:   art.ID,
			RunID:        run.ID,
			JobID:        jobs[0].ID,
		}, nil
	})
}

func pickExampleArtifact(arts []github.Artifact, preferred string) github.Artifact {
	for _, a := range arts {
		if a.Name == preferred {
			return a
		}
	}
	return arts[0]
}

func applyIndexExampleExtras(ex workflowExample, ids indexExampleIDs, abs func(string) string, extra map[string]any) {
	extra["ExampleArtifactGitHub"] = fmt.Sprintf(
		"https://github.com/%s/%s/suites/%d/artifacts/%d",
		ex.owner, ex.repo, ids.CheckSuiteID, ids.ArtifactID,
	)
	extra["ExampleArtifactDest"] = abs(fmt.Sprintf("/%s/%s/actions/artifacts/%d", ex.owner, ex.repo, ids.ArtifactID))
	extra["ExampleRunGitHub"] = fmt.Sprintf("https://github.com/%s/%s/actions/runs/%d", ex.owner, ex.repo, ids.RunID)
	extra["ExampleRunDest"] = abs(fmt.Sprintf("/%s/%s/actions/runs/%d", ex.owner, ex.repo, ids.RunID))
	extra["ExampleJobGitHub"] = fmt.Sprintf(
		"https://github.com/%s/%s/runs/%s?check_suite_focus=true",
		ex.owner, ex.repo, strconv.FormatInt(ids.JobID, 10),
	)
	extra["ExampleJobDest"] = abs(fmt.Sprintf("/%s/%s/runs/%d", ex.owner, ex.repo, ids.JobID))
}
