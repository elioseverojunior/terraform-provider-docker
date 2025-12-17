package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/elioseverojunior/terraform-provider-docker/internal/docker"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &ContainerDataSource{}

type ContainerDataSource struct {
	client *docker.Client
}

type ContainerDataSourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Image       types.String `tfsdk:"image"`
	ContainerID types.String `tfsdk:"container_id"`
	Status      types.String `tfsdk:"status"`
	State       types.String `tfsdk:"state"`
	IPAddress   types.String `tfsdk:"ip_address"`
	Gateway     types.String `tfsdk:"gateway"`
	Labels      types.Map    `tfsdk:"labels"`
	Env         types.List   `tfsdk:"env"`
	Command     types.List   `tfsdk:"command"`
}

func NewContainerDataSource() datasource.DataSource {
	return &ContainerDataSource{}
}

func (d *ContainerDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_container"
}

func (d *ContainerDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Use this data source to get information about a Docker container.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The ID of this resource.",
				Computed:    true,
			},
			"name": schema.StringAttribute{
				Description: "The name of the Docker container.",
				Required:    true,
			},
			"image": schema.StringAttribute{
				Description: "The image used by the container.",
				Computed:    true,
			},
			"container_id": schema.StringAttribute{
				Description: "The Docker container ID.",
				Computed:    true,
			},
			"status": schema.StringAttribute{
				Description: "The status of the container (e.g., running, exited).",
				Computed:    true,
			},
			"state": schema.StringAttribute{
				Description: "The state of the container.",
				Computed:    true,
			},
			"ip_address": schema.StringAttribute{
				Description: "The IP address of the container.",
				Computed:    true,
			},
			"gateway": schema.StringAttribute{
				Description: "The network gateway of the container.",
				Computed:    true,
			},
			"labels": schema.MapAttribute{
				Description: "User-defined key/value metadata.",
				Computed:    true,
				ElementType: types.StringType,
			},
			"env": schema.ListAttribute{
				Description: "Environment variables set in the container.",
				Computed:    true,
				ElementType: types.StringType,
			},
			"command": schema.ListAttribute{
				Description: "The command used to start the container.",
				Computed:    true,
				ElementType: types.StringType,
			},
		},
	}
}

func (d *ContainerDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *ContainerDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data ContainerDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	containerName := data.Name.ValueString()

	containerJSON, err := d.client.ContainerInspect(ctx, containerName)
	if err != nil {
		if strings.Contains(err.Error(), "No such container") || strings.Contains(err.Error(), "not found") {
			resp.Diagnostics.AddError("Container Not Found", fmt.Sprintf("Container %s not found", containerName))
			return
		}
		resp.Diagnostics.AddError("Container Read Error", fmt.Sprintf("Unable to read container %s: %s", containerName, err))
		return
	}

	data.ID = types.StringValue(containerJSON.ID)
	data.Name = types.StringValue(strings.TrimPrefix(containerJSON.Name, "/"))
	data.Image = types.StringValue(containerJSON.Config.Image)
	data.ContainerID = types.StringValue(containerJSON.ID)

	// State
	if containerJSON.State != nil {
		data.Status = types.StringValue(containerJSON.State.Status)
		if containerJSON.State.Running {
			data.State = types.StringValue("running")
		} else {
			data.State = types.StringValue("stopped")
		}
	}

	// Network info
	if containerJSON.NetworkSettings != nil {
		if containerJSON.NetworkSettings.IPAddress != "" {
			data.IPAddress = types.StringValue(containerJSON.NetworkSettings.IPAddress)
		} else {
			data.IPAddress = types.StringNull()
		}
		if containerJSON.NetworkSettings.Gateway != "" {
			data.Gateway = types.StringValue(containerJSON.NetworkSettings.Gateway)
		} else {
			data.Gateway = types.StringNull()
		}
	}

	// Labels
	if len(containerJSON.Config.Labels) > 0 {
		labels, diags := types.MapValueFrom(ctx, types.StringType, containerJSON.Config.Labels)
		resp.Diagnostics.Append(diags...)
		data.Labels = labels
	} else {
		data.Labels = types.MapNull(types.StringType)
	}

	// Environment variables
	if len(containerJSON.Config.Env) > 0 {
		env, diags := types.ListValueFrom(ctx, types.StringType, containerJSON.Config.Env)
		resp.Diagnostics.Append(diags...)
		data.Env = env
	} else {
		data.Env = types.ListNull(types.StringType)
	}

	// Command
	if len(containerJSON.Config.Cmd) > 0 {
		cmd, diags := types.ListValueFrom(ctx, types.StringType, containerJSON.Config.Cmd)
		resp.Diagnostics.Append(diags...)
		data.Command = cmd
	} else {
		data.Command = types.ListNull(types.StringType)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
