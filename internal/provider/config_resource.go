package provider

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/docker/docker/api/types/swarm"
	"github.com/elioseverojunior/terraform-provider-docker/internal/docker"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	tftypes "github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var (
	_ resource.Resource                = &ConfigResource{}
	_ resource.ResourceWithImportState = &ConfigResource{}
)

type ConfigResource struct {
	client *docker.Client
}

type ConfigResourceModel struct {
	ID     tftypes.String `tfsdk:"id"`
	Name   tftypes.String `tfsdk:"name"`
	Data   tftypes.String `tfsdk:"data"`
	Labels tftypes.Map    `tfsdk:"labels"`
}

func NewConfigResource() resource.Resource {
	return &ConfigResource{}
}

func (r *ConfigResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_config"
}

func (r *ConfigResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages Docker Swarm configs. Requires Docker to be in Swarm mode.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The ID of this resource.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "The name of the Docker config.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"data": schema.StringAttribute{
				Description: "Base64-encoded config data.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"labels": schema.MapAttribute{
				Description: "User-defined key/value metadata.",
				Optional:    true,
				ElementType: tftypes.StringType,
			},
		},
	}
}

func (r *ConfigResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	providerData, ok := req.ProviderData.(*ProviderData)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *ProviderData, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	r.client = providerData.DockerClient
}

func (r *ConfigResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data ConfigResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Decode base64 data
	configData, err := base64.StdEncoding.DecodeString(data.Data.ValueString())
	if err != nil {
		// Try URL-safe base64 encoding as fallback
		configData, err = base64.URLEncoding.DecodeString(data.Data.ValueString())
		if err != nil {
			resp.Diagnostics.AddError(
				"Invalid Config Data",
				fmt.Sprintf("Failed to decode base64 config data: %s", err),
			)
			return
		}
	}

	// Convert labels
	labels := make(map[string]string)
	if !data.Labels.IsNull() {
		resp.Diagnostics.Append(data.Labels.ElementsAs(ctx, &labels, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	configSpec := swarm.ConfigSpec{
		Annotations: swarm.Annotations{
			Name:   data.Name.ValueString(),
			Labels: labels,
		},
		Data: configData,
	}

	tflog.Debug(ctx, "Creating Docker config", map[string]interface{}{
		"name": data.Name.ValueString(),
	})

	configResponse, err := r.client.ConfigCreate(ctx, configSpec)
	if err != nil {
		resp.Diagnostics.AddError(
			"Docker Config Creation Failed",
			fmt.Sprintf("Failed to create config %s: %s. Note: Docker must be in Swarm mode.", data.Name.ValueString(), err),
		)
		return
	}

	data.ID = tftypes.StringValue(configResponse.ID)

	tflog.Debug(ctx, "Created Docker config", map[string]interface{}{
		"id":   configResponse.ID,
		"name": data.Name.ValueString(),
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ConfigResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data ConfigResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	config, _, err := r.client.ConfigInspectWithRaw(ctx, data.ID.ValueString())
	if err != nil {
		if strings.Contains(err.Error(), "No such config") || strings.Contains(err.Error(), "config not found") {
			tflog.Debug(ctx, "Config not found, removing from state", map[string]interface{}{
				"id": data.ID.ValueString(),
			})
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Docker Config Read Failed",
			fmt.Sprintf("Failed to read config %s: %s", data.ID.ValueString(), err),
		)
		return
	}

	data.Name = tftypes.StringValue(config.Spec.Name)

	// Config data is available in Spec.Data
	if len(config.Spec.Data) > 0 {
		data.Data = tftypes.StringValue(base64.StdEncoding.EncodeToString(config.Spec.Data))
	}

	if len(config.Spec.Labels) > 0 {
		labels, diags := tftypes.MapValueFrom(ctx, tftypes.StringType, config.Spec.Labels)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		data.Labels = labels
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ConfigResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data ConfigResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Get current config to get version
	config, _, err := r.client.ConfigInspectWithRaw(ctx, data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Docker Config Update Failed",
			fmt.Sprintf("Failed to inspect config %s: %s", data.ID.ValueString(), err),
		)
		return
	}

	// Convert labels
	labels := make(map[string]string)
	if !data.Labels.IsNull() {
		resp.Diagnostics.Append(data.Labels.ElementsAs(ctx, &labels, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	// Only labels can be updated, data changes require replacement
	configSpec := swarm.ConfigSpec{
		Annotations: swarm.Annotations{
			Name:   data.Name.ValueString(),
			Labels: labels,
		},
		Data: config.Spec.Data, // Keep existing data
	}

	err = r.client.ConfigUpdate(ctx, data.ID.ValueString(), config.Version, configSpec)
	if err != nil {
		resp.Diagnostics.AddError(
			"Docker Config Update Failed",
			fmt.Sprintf("Failed to update config %s: %s", data.ID.ValueString(), err),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ConfigResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data ConfigResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Deleting Docker config", map[string]interface{}{
		"id":   data.ID.ValueString(),
		"name": data.Name.ValueString(),
	})

	err := r.client.ConfigRemove(ctx, data.ID.ValueString())
	if err != nil {
		if strings.Contains(err.Error(), "No such config") || strings.Contains(err.Error(), "config not found") {
			tflog.Debug(ctx, "Config already removed", map[string]interface{}{
				"id": data.ID.ValueString(),
			})
			return
		}
		resp.Diagnostics.AddError(
			"Docker Config Deletion Failed",
			fmt.Sprintf("Failed to delete config %s: %s", data.ID.ValueString(), err),
		)
		return
	}

	tflog.Debug(ctx, "Deleted Docker config", map[string]interface{}{
		"id": data.ID.ValueString(),
	})
}

func (r *ConfigResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
