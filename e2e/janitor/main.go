// Command janitor deletes fixture projects leaked by interrupted E2E runs.
//
// Every project the E2E suite creates is named gl-e2e-* (see the
// projectNamePrefix constant in package e2e) and lives under $GL_E2E_GROUP.
// Scripts self-clean via deferred deletes, but a SIGKILLed run leaves its
// projects behind; this sweeper removes those matching the prefix and older
// than -max-age.
//
// On instances with delayed project deletion enabled, deleted projects
// linger in a pending state (renamed *-deletion_scheduled-*) until GitLab
// purges them. Those are skipped by default — GitLab already owns their
// removal — and permanently removed with -hard.
//
// Usage:
//
//	GL_E2E_HOST=... GL_E2E_TOKEN=... GL_E2E_GROUP=... go run ./e2e/janitor [-max-age 1h] [-dry-run] [-hard]
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

const projectNamePrefix = "gl-e2e-"

func main() {
	maxAge := flag.Duration("max-age", time.Hour, "only delete projects older than this")
	dryRun := flag.Bool("dry-run", false, "list matching projects without deleting")
	hard := flag.Bool("hard", false, "permanently remove projects already pending deletion")
	flag.Parse()

	if err := run(*maxAge, *dryRun, *hard); err != nil {
		fmt.Fprintln(os.Stderr, "janitor:", err)
		os.Exit(1)
	}
}

func run(maxAge time.Duration, dryRun, hard bool) error {
	host := os.Getenv("GL_E2E_HOST")
	token := os.Getenv("GL_E2E_TOKEN")
	group := os.Getenv("GL_E2E_GROUP")
	if host == "" || token == "" || group == "" {
		return fmt.Errorf("GL_E2E_HOST, GL_E2E_TOKEN, and GL_E2E_GROUP must be set")
	}

	client, err := gitlab.NewClient(token, gitlab.WithBaseURL(host))
	if err != nil {
		return err
	}

	cutoff := time.Now().Add(-maxAge)
	swept := 0
	pending := 0

	opts := &gitlab.ListGroupProjectsOptions{
		Search:      gitlab.Ptr(projectNamePrefix),
		ListOptions: gitlab.ListOptions{PerPage: 100},
	}
	for {
		projects, resp, err := client.Groups.ListGroupProjects(group, opts)
		if err != nil {
			return fmt.Errorf("list projects in group %q: %w", group, err)
		}

		for _, project := range projects {
			if !strings.HasPrefix(project.Path, projectNamePrefix) {
				continue
			}

			// Already marked for deletion: GitLab purges these on its own
			// schedule. Only -hard touches them, with the permanent-removal
			// form of the delete API.
			if project.MarkedForDeletionOn != nil {
				if !hard {
					pending++
					continue
				}
				if dryRun {
					fmt.Println("would permanently remove", project.PathWithNamespace)
					swept++
					continue
				}
				_, err := client.Projects.DeleteProject(project.ID, &gitlab.DeleteProjectOptions{
					FullPath:          gitlab.Ptr(project.PathWithNamespace),
					PermanentlyRemove: gitlab.Ptr(true),
				})
				if err != nil {
					fmt.Fprintf(os.Stderr, "janitor: permanently remove %s: %v\n", project.PathWithNamespace, err)
					continue
				}
				fmt.Println("permanently removed", project.PathWithNamespace)
				swept++
				continue
			}

			if project.CreatedAt != nil && project.CreatedAt.After(cutoff) {
				continue
			}

			if dryRun {
				fmt.Println("would delete", project.PathWithNamespace)
				swept++
				continue
			}
			if _, err := client.Projects.DeleteProject(project.ID, nil); err != nil {
				fmt.Fprintf(os.Stderr, "janitor: delete %s: %v\n", project.PathWithNamespace, err)
				continue
			}
			fmt.Println("deleted", project.PathWithNamespace)
			swept++
		}

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	if pending > 0 {
		fmt.Printf("skipped %d project(s) already pending deletion (GitLab purges them on schedule; pass -hard to remove now)\n", pending)
	}
	fmt.Printf("swept %d project(s)\n", swept)

	return nil
}
