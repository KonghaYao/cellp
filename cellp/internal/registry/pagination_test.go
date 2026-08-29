package registry

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

func TestListProjectsPagination(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "paginate.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		if _, err := s.CreateProject(ctx, CreateProjectInput{ID: fmt.Sprintf("p%d", i)}); err != nil {
			t.Fatal(err)
		}
		time.Sleep(time.Millisecond)
	}

	page1, err := s.ListProjects(ctx, ListProjectsOpts{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(page1.Projects) != 2 || page1.NextCursor == "" {
		t.Fatalf("page1: %+v", page1)
	}

	page2, err := s.ListProjects(ctx, ListProjectsOpts{Limit: 2, Cursor: page1.NextCursor})
	if err != nil {
		t.Fatal(err)
	}
	if len(page2.Projects) != 2 || page2.NextCursor == "" {
		t.Fatalf("page2: %+v", page2)
	}

	page3, err := s.ListProjects(ctx, ListProjectsOpts{Limit: 2, Cursor: page2.NextCursor})
	if err != nil {
		t.Fatal(err)
	}
	if len(page3.Projects) != 1 || page3.NextCursor != "" {
		t.Fatalf("page3: %+v", page3)
	}

	seen := map[string]bool{}
	for _, page := range []*ListProjectsPage{page1, page2, page3} {
		for _, p := range page.Projects {
			if seen[p.ID] {
				t.Fatalf("duplicate project %s", p.ID)
			}
			seen[p.ID] = true
		}
	}
}

func TestListVersionsPaginationAndFilters(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "versions.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	_, _ = s.CreateProject(ctx, CreateProjectInput{ID: "demo"})

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 4; i++ {
		id := fmt.Sprintf("v%d", i)
		if _, err := s.CreateVersion(ctx, CreateVersionInput{ID: id, ProjectID: "demo"}); err != nil {
			t.Fatal(err)
		}
		status := StatusPending
		if i%2 == 0 {
			status = StatusReady
		}
		_ = s.UpdateVersionStatus(ctx, "demo", id, status, nil)
		created := base.Add(time.Duration(i) * time.Hour).Format(time.RFC3339Nano)
		_, _ = s.db.Exec(`UPDATE versions SET created_at = ?, updated_at = ? WHERE project_id = ? AND id = ?`,
			created, created, "demo", id)
	}

	page1, err := s.ListVersions(ctx, "demo", ListVersionsOpts{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(page1.Versions) != 2 || page1.NextCursor == "" {
		t.Fatalf("page1: %+v", page1)
	}
	if page1.Versions[0].ID != "v3" || page1.Versions[1].ID != "v2" {
		t.Fatalf("expected newest first: %+v", page1.Versions)
	}

	page2, err := s.ListVersions(ctx, "demo", ListVersionsOpts{Limit: 2, Cursor: page1.NextCursor})
	if err != nil {
		t.Fatal(err)
	}
	if len(page2.Versions) != 2 || page2.NextCursor != "" {
		t.Fatalf("page2: %+v", page2)
	}

	readyOnly, err := s.ListVersions(ctx, "demo", ListVersionsOpts{Limit: 10, Status: StatusReady})
	if err != nil {
		t.Fatal(err)
	}
	if len(readyOnly.Versions) != 2 {
		t.Fatalf("ready filter: %+v", readyOnly.Versions)
	}

	since := base.Add(2 * time.Hour)
	sincePage, err := s.ListVersions(ctx, "demo", ListVersionsOpts{Limit: 10, Since: &since})
	if err != nil {
		t.Fatal(err)
	}
	if len(sincePage.Versions) != 2 {
		t.Fatalf("since filter: %+v", sincePage.Versions)
	}

	count, err := s.CountVersions(ctx, "demo")
	if err != nil || count != 4 {
		t.Fatalf("count = %d err=%v", count, err)
	}
}

func TestListProjectsQueryFilter(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "query.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()

	for _, id := range []string{"alpha-app", "beta-app", "cellp-dashboard", "demo-app"} {
		if _, err := s.CreateProject(ctx, CreateProjectInput{ID: id}); err != nil {
			t.Fatal(err)
		}
	}

	matches, err := s.ListProjects(ctx, ListProjectsOpts{Limit: 10, Query: "cellp"})
	if err != nil {
		t.Fatal(err)
	}
	if len(matches.Projects) != 1 || matches.Projects[0].ID != "cellp-dashboard" {
		t.Fatalf("query filter: %+v", matches.Projects)
	}

	paged, err := s.ListProjects(ctx, ListProjectsOpts{Limit: 1, Query: "app"})
	if err != nil {
		t.Fatal(err)
	}
	if len(paged.Projects) != 1 || paged.NextCursor == "" {
		t.Fatalf("paged query: %+v", paged)
	}

	page2, err := s.ListProjects(ctx, ListProjectsOpts{Limit: 1, Query: "app", Cursor: paged.NextCursor})
	if err != nil {
		t.Fatal(err)
	}
	if len(page2.Projects) != 1 {
		t.Fatalf("page2 query: %+v", page2)
	}
}

func TestListProjectsVersionCount(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "counts.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	_, _ = s.CreateProject(ctx, CreateProjectInput{ID: "demo"})
	for i := 0; i < 3; i++ {
		_, _ = s.CreateVersion(ctx, CreateVersionInput{
			ID: fmt.Sprintf("v%d", i), ProjectID: "demo",
		})
	}
	page, err := s.ListProjects(ctx, ListProjectsOpts{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Projects) != 1 || page.Projects[0].VersionCount != 3 {
		t.Fatalf("page: %+v", page)
	}
}
