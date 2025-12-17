package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/docker/docker/api/types/volume"
	"github.com/elioseverojunior/terraform-provider-docker/internal/docker"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var (
	_ resource.Resource                = &VolumeResource{}
	_ resource.ResourceWithImportState = &VolumeResource{}
)

type VolumeResource struct {
	client *docker.Client
}

type VolumeResourceModel struct {
	ID         types.String `tfsdk:"id"`
	Name       types.String `tfsdk:"name"`
	Driver     types.String `tfsdk:"driver"`
	DriverOpts types.Map    `tfsdk:"driver_opts"`
	Labels     types.Map    `tfsdk:"labels"`
	Mountpoint types.String `tfsdk:"mountpoint"`
	Force      types.Bool   `tfsdk:"force"`
}

func NewVolumeResource() resource.Resource {
	return &VolumeResource{}
}

func (r *VolumeResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_volume"
}

func (r *VolumeResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages Docker volumes.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The ID of this resource.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "The name of the Docker volume.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"driver": schema.StringAttribute{
				Description: "The driver that this volume uses. Default is 'local'.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString("local"),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"driver_opts": schema.MapAttribute{
				Description: "Options specific to the volume driver.",
				Optional:    true,
				ElementType: types.StringType,
			},
			"labels": schema.MapAttribute{
				Description: "User-defined key/value metadata.",
				Optional:    true,
				ElementType: types.StringType,
			},
			"mountpoint": schema.StringAttribute{
				Description: "The mount point of the volume on the host.",
				Computed:    true,
			},
			"force": schema.BoolAttribute{
				Description: "If true, forces the removal of the volume even if it's in use. Default is false.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
		},
	}
}

func (r *VolumeResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *VolumeResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data VolumeResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	volumeName := data.Name.ValueString()
	tflog.Debug(ctx, "Creating Docker volume", map[string]interface{}{
		"name": volumeName,
	})

	createOptions := volume.CreateOptions{
		Name:   volumeName,
		Driver: data.Driver.ValueString(),
	}

	// Driver options
	if !data.DriverOpts.IsNull() {
		driverOpts := make(map[string]string)
		resp.Diagnostics.Append(data.DriverOpts.ElementsAs(ctx, &driverOpts, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		createOptions.DriverOpts = driverOpts
	}

	// Labels
	if !data.Labels.IsNull() {
		labels := make(map[string]string)
		resp.Diagnostics.Append(data.Labels.ElementsAs(ctx, &labels, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		createOptions.Labels = labels
	}

	volumeResp, err := r.client.VolumeCreate(ctx, createOptions)
	if err != nil {
		resp.Diagnostics.AddError("Volume Create Error", fmt.Sprintf("Unable to create volume %s: %s", volumeName, err))
		return
	}

	data.ID = types.StringValue(volumeResp.Name)
	data.Mountpoint = types.StringValue(volumeResp.Mountpoint)

	tflog.Debug(ctx, "Created Docker volume", map[string]interface{}{
		"name":       volumeName,
		"mountpoint": volumeResp.Mountpoint,
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *VolumeResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data VolumeResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	volumeName := data.Name.ValueString()

	volumeInspect, err := r.client.VolumeInspect(ctx, volumeName)
	if err != nil {
		if strings.Contains(err.Error(), "no such volume") || strings.Contains(err.Error(), "not found") {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Volume Read Error", fmt.Sprintf("Unable to read volume %s: %s", volumeName, err))
		return
	}

	data.Name = types.StringValue(volumeInspect.Name)
	data.Driver = types.StringValue(volumeInspect.Driver)
	data.Mountpoint = types.StringValue(volumeInspect.Mountpoint)

	// Driver options
	if len(volumeInspect.Options) > 0 {
		driverOpts, diags := types.MapValueFrom(ctx, types.StringType, volumeInspect.Options)
		resp.Diagnostics.Append(diags...)
		data.DriverOpts = driverOpts
	}

	// Labels
	if len(volumeInspect.Labels) > 0 {
		labels, diags := types.MapValueFrom(ctx, types.StringType, volumeInspect.Labels)
		resp.Diagnostics.Append(diags...)
		data.Labels = labels
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *VolumeResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data VolumeResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Docker volumes are immutable, only labels can potentially change
	// The Docker API doesn't support updating volumes directly
	// For now, we just save the state as-is

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *VolumeResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data VolumeResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	volumeName := data.Name.ValueString()

	tflog.Debug(ctx, "Deleting Docker volume", map[string]interface{}{
		"name": volumeName,
	})

	err := r.client.VolumeRemove(ctx, volumeName, data.Force.ValueBool())
	if err != nil {
		if strings.Contains(err.Error(), "no such volume") || strings.Contains(err.Error(), "not found") {
			return
		}
		resp.Diagnostics.AddError("Volume Delete Error", fmt.Sprintf("Unable to delete volume %s: %s", volumeName, err))
		return
	}

	tflog.Debug(ctx, "Deleted Docker volume", map[string]interface{}{
		"name": volumeName,
	})
}

func (r *VolumeResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("name"), req, resp)
}
