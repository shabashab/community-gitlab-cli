//go:build e2e

package e2e

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

// projectNamePrefix marks every project the suite creates. The janitor
// (e2e/janitor) deletes group projects with this prefix, so keep the two in
// sync.
const projectNamePrefix = "gl-e2e-"

func newGitLabClient() (*gitlab.Client, error) {
	return gitlab.NewClient(
		os.Getenv(envToken),
		gitlab.WithBaseURL(os.Getenv(envHost)),
	)
}

// createProject creates a private fixture project under $GL_E2E_GROUP and
// waits until the API serves it, so scripts can push immediately.
func createProject(name string) (*gitlab.Project, error) {
	client, err := newGitLabClient()
	if err != nil {
		return nil, err
	}

	group, _, err := client.Groups.GetGroup(os.Getenv(envGroup), nil)
	if err != nil {
		return nil, fmt.Errorf("resolve group %q: %w", os.Getenv(envGroup), err)
	}

	project, _, err := client.Projects.CreateProject(&gitlab.CreateProjectOptions{
		Name:        gitlab.Ptr(name),
		Path:        gitlab.Ptr(name),
		NamespaceID: gitlab.Ptr(group.ID),
		Visibility:  gitlab.Ptr(gitlab.PrivateVisibility),
	})
	if err != nil {
		return nil, fmt.Errorf("create project %q: %w", name, err)
	}

	for attempt := 0; attempt < 10; attempt++ {
		if _, _, err = client.Projects.GetProject(project.ID, nil); err == nil {
			return project, nil
		}
		time.Sleep(300 * time.Millisecond)
	}

	return nil, fmt.Errorf("project %q not ready: %w", name, err)
}

func deleteProject(path string) error {
	client, err := newGitLabClient()
	if err != nil {
		return err
	}

	_, err = client.Projects.DeleteProject(path, nil)
	if err != nil && !isNotFound(err) && !isAlreadyMarkedForDeletion(err) {
		return fmt.Errorf("delete project %q: %w", path, err)
	}

	return nil
}

// isAlreadyMarkedForDeletion recognizes the 400 an instance with delayed
// project deletion returns when a project was already deleted once — for the
// suite that outcome is success.
func isAlreadyMarkedForDeletion(err error) bool {
	var respErr *gitlab.ErrorResponse
	return errors.As(err, &respErr) && respErr.StatusCode == 400 &&
		strings.Contains(err.Error(), "marked for deletion")
}

func deleteProjectBySuffix(suffix string) error {
	return deleteProject(os.Getenv(envGroup) + "/" + projectNamePrefix + suffix)
}

func isNotFound(err error) bool {
	var respErr *gitlab.ErrorResponse
	return errors.As(err, &respErr) && respErr.StatusCode == 404
}
