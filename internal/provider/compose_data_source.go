package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/elioseverojunior/terraform-provider-docker/internal/docker"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	tftypes "github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &ComposeDataSource{}

type ComposeDataSource struct {
	client *docker.Client
}

type ComposeDataSourceModel struct {
	ID              tftypes.String       `tfsdk:"id"`
	ProjectName     tftypes.String       `tfsdk:"project_name"`
	Services        []ComposeServiceInfo `tfsdk:"services"`
	RunningServices tftypes.Int64        `tfsdk:"running_services"`
	TotalServices   tftypes.Int64        `tfsdk:"total_services"`
}

type ComposeServiceInfo struct {
	Name        tftypes.String `tfsdk:"name"`
	ContainerID tftypes.String `tfsdk:"container_id"`
	State       tftypes.String `tfsdk:"state"`
	Status      tftypes.String `tfsdk:"status"`
	Health      tftypes.String `tfsdk:"health"`
	Image       tftypes.String `tfsdk:"image"`
	Ports       tftypes.String `tfsdk:"ports"`
}

func NewComposeDataSource() datasource.DataSource {
	return &ComposeDataSource{}
}

func (d *ComposeDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_compose"
}

func (d *ComposeDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Use this data source to get information about a Docker Compose stack.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The ID of this resource.",
				Computed:    true,
			},
			"project_name": schema.StringAttribute{
				Description: "The name of the Docker Compose project.",
				Required:    true,
			},
			"running_services": schema.Int64Attribute{
				Description: "Number of running services in the stack.",
				Computed:    true,
			},
			"total_services": schema.Int64Attribute{
				Description: "Total number of services in the stack.",
				Computed:    true,
			},
		},
		Blocks: map[string]schema.Block{
			"services": schema.ListNestedBlock{
				Description: "List of services in the compose stack.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Description: "Service name.",
							Computed:    true,
						},
						"container_id": schema.StringAttribute{
							Description: "Container ID.",
							Computed:    true,
						},
						"state": schema.StringAttribute{
							Description: "Container state (running, exited, etc.).",
							Computed:    true,
						},
						"status": schema.StringAttribute{
							Description: "Container status.",
							Computed:    true,
						},
						"health": schema.StringAttribute{
							Description: "Container health status.",
							Computed:    true,
						},
						"image": schema.StringAttribute{
							Description: "Image used by the service.",
							Computed:    true,
						},
						"ports": schema.StringAttribute{
							Description: "Port mappings.",
							Computed:    true,
						},
					},
				},
			},
		},
	}
}

func (d *ComposeDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	providerData, ok := req.ProviderData.(*ProviderData)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *ProviderData, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	d.client = providerData.DockerClient
}

func (d *ComposeDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data ComposeDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	projectName := data.ProjectName.ValueString()

	// Query containers by compose project label using Docker API
	filterArgs := filters.NewArgs()
	filterArgs.Add("label", fmt.Sprintf("com.docker.compose.project=%s", projectName))

	containers, err := d.client.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: filterArgs,
	})
	if err != nil {
		resp.Diagnostics.AddError(
			"Docker API Error",
			fmt.Sprintf("Failed to list containers for project %s: %s", projectName, err),
		)
		return
	}

	data.ID = tftypes.StringValue(projectName)

	// Track unique services
	serviceMap := make(map[string]ComposeServiceInfo)
	runningCount := int64(0)

	for _, container := range containers {
		// Get service name from label
		serviceName := container.Labels["com.docker.compose.service"]
		if serviceName == "" {
			continue
		}

		// Build port string
		var portStrings []string
		for _, port := range container.Ports {
			if port.PublicPort > 0 {
				portStrings = append(portStrings, fmt.Sprintf("%s:%d->%d/%s",
					port.IP, port.PublicPort, port.PrivatePort, port.Type))
			} else {
				portStrings = append(portStrings, fmt.Sprintf("%d/%s",
					port.PrivatePort, port.Type))
			}
		}

		// Get health status
		health := ""
		if container.State == "running" {
			// Check if container has health status in status string
			if strings.Contains(container.Status, "(healthy)") {
				health = "healthy"
			} else if strings.Contains(container.Status, "(unhealthy)") {
				health = "unhealthy"
			} else if strings.Contains(container.Status, "(starting)") {
				health = "starting"
			}
		}

		service := ComposeServiceInfo{
			Name:        tftypes.StringValue(serviceName),
			ContainerID: tftypes.StringValue(container.ID[:12]),
			State:       tftypes.StringValue(container.State),
			Status:      tftypes.StringValue(container.Status),
			Image:       tftypes.StringValue(container.Image),
			Ports:       tftypes.StringValue(strings.Join(portStrings, ", ")),
		}

		if health != "" {
			service.Health = tftypes.StringValue(health)
		} else {
			service.Health = tftypes.StringNull()
		}

		// Use the latest container for each service (in case of replicas)
		if existing, ok := serviceMap[serviceName]; ok {
			// Keep the running one if available
			if container.State == "running" && existing.State.ValueString() != "running" {
				serviceMap[serviceName] = service
			}
		} else {
			serviceMap[serviceName] = service
		}

		if container.State == "running" {
			runningCount++
		}
	}

	// Convert map to slice
	services := make([]ComposeServiceInfo, 0, len(serviceMap))
	for _, service := range serviceMap {
		services = append(services, service)
	}

	data.Services = services
	data.TotalServices = tftypes.Int64Value(int64(len(services)))
	data.RunningServices = tftypes.Int64Value(runningCount)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
