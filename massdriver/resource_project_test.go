package massdriver

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"terraform-provider-massdriver/massdriver/services/projects"
)

// Mock Project Service for testing
type MockProjectService struct {
	projects map[string]*projects.Project
}

func NewMockProjectService() *MockProjectService {
	return &MockProjectService{
		projects: make(map[string]*projects.Project),
	}
}

func (m *MockProjectService) CreateProject(ctx context.Context, project *projects.Project) (*projects.Project, error) {
	project.ID = "test-project-id-123"
	m.projects[project.ID] = project
	return project, nil
}

func (m *MockProjectService) GetProject(ctx context.Context, idOrSlug string) (*projects.Project, error) {
	if project, ok := m.projects[idOrSlug]; ok {
		return project, nil
	}
	return nil, fmt.Errorf("not found")
}

func (m *MockProjectService) UpdateProject(ctx context.Context, project *projects.Project) (*projects.Project, error) {
	if _, ok := m.projects[project.ID]; !ok {
		return nil, fmt.Errorf("not found")
	}
	m.projects[project.ID] = project
	return project, nil
}

func (m *MockProjectService) DeleteProject(ctx context.Context, idOrSlug string) error {
	if _, ok := m.projects[idOrSlug]; !ok {
		return fmt.Errorf("not found")
	}
	delete(m.projects, idOrSlug)
	return nil
}

func TestAccMassdriverProjectBasic(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:  func() { testAccPreCheck(t) },
		Providers: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: testAccCheckMassdriverProjectConfigBasic(),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMassdriverProjectExists("massdriver_project.test"),
					resource.TestCheckResourceAttr("massdriver_project.test", "name", "Test Project"),
					resource.TestCheckResourceAttr("massdriver_project.test", "slug", "test-project"),
					resource.TestCheckResourceAttr("massdriver_project.test", "description", "A test project"),
				),
			},
		},
	})
}

func TestAccMassdriverProjectUpdate(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:  func() { testAccPreCheck(t) },
		Providers: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: testAccCheckMassdriverProjectConfigBasic(),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMassdriverProjectExists("massdriver_project.test"),
					resource.TestCheckResourceAttr("massdriver_project.test", "name", "Test Project"),
				),
			},
			{
				Config: testAccCheckMassdriverProjectConfigUpdate(),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMassdriverProjectExists("massdriver_project.test"),
					resource.TestCheckResourceAttr("massdriver_project.test", "name", "Updated Test Project"),
					resource.TestCheckResourceAttr("massdriver_project.test", "description", "An updated test project"),
				),
			},
		},
	})
}

func testAccCheckMassdriverProjectConfigBasic() string {
	return `
	resource "massdriver_project" "test" {
		name        = "Test Project"
		slug        = "test-project"
		description = "A test project"
	}
	`
}

func testAccCheckMassdriverProjectConfigUpdate() string {
	return `
	resource "massdriver_project" "test" {
		name        = "Updated Test Project"
		slug        = "test-project"
		description = "An updated test project"
	}
	`
}

func testAccCheckMassdriverProjectExists(n string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]

		if !ok {
			return fmt.Errorf("Not found: %s", n)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("No ID set")
		}

		return nil
	}
}

// Unit tests with mocks
func TestResourceProjectCreate(t *testing.T) {
	mockService := NewMockProjectService()
	ctx := context.Background()

	// Mock the service call
	project := &projects.Project{
		Name:        "Test Project",
		Slug:        "test-project",
		Description: "A test project",
	}

	result, err := mockService.CreateProject(ctx, project)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if result.ID == "" {
		t.Fatal("Expected ID to be set")
	}

	if result.Name != "Test Project" {
		t.Errorf("Expected name 'Test Project', got '%s'", result.Name)
	}
}

func TestResourceProjectRead(t *testing.T) {
	mockService := NewMockProjectService()
	ctx := context.Background()

	// Create a project first
	project := &projects.Project{
		Name:        "Test Project",
		Slug:        "test-project",
		Description: "A test project",
	}

	created, err := mockService.CreateProject(ctx, project)
	if err != nil {
		t.Fatalf("Failed to create project: %v", err)
	}

	// Now read it
	result, err := mockService.GetProject(ctx, created.ID)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if result.Name != "Test Project" {
		t.Errorf("Expected name 'Test Project', got '%s'", result.Name)
	}
}

func TestResourceProjectUpdate(t *testing.T) {
	mockService := NewMockProjectService()
	ctx := context.Background()

	// Create a project first
	project := &projects.Project{
		Name:        "Test Project",
		Slug:        "test-project",
		Description: "A test project",
	}

	created, err := mockService.CreateProject(ctx, project)
	if err != nil {
		t.Fatalf("Failed to create project: %v", err)
	}

	// Update it
	created.Name = "Updated Project"
	created.Description = "Updated description"

	updated, err := mockService.UpdateProject(ctx, created)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if updated.Name != "Updated Project" {
		t.Errorf("Expected name 'Updated Project', got '%s'", updated.Name)
	}
}

func TestResourceProjectDelete(t *testing.T) {
	mockService := NewMockProjectService()
	ctx := context.Background()

	// Create a project first
	project := &projects.Project{
		Name:        "Test Project",
		Slug:        "test-project",
		Description: "A test project",
	}

	created, err := mockService.CreateProject(ctx, project)
	if err != nil {
		t.Fatalf("Failed to create project: %v", err)
	}

	// Delete it
	err = mockService.DeleteProject(ctx, created.ID)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Verify it's gone
	_, err = mockService.GetProject(ctx, created.ID)
	if err == nil {
		t.Fatal("Expected error when getting deleted project")
	}
}

