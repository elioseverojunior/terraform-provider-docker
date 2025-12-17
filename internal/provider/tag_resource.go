package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/docker/docker/api/types/image"
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
	_ resource.Resource                = &TagResource{}
	_ resource.ResourceWithImportState = &TagResource{}
)

type TagResource struct {
	client *docker.Client
}

type TagResourceModel struct {
	ID            tftypes.String `tfsdk:"id"`
	SourceImage   tftypes.String `tfsdk:"source_image"`
	TargetImage   tftypes.String `tfsdk:"target_image"`
	SourceImageID tftypes.String `tfsdk:"source_image_id"`
}

func NewTagResource() resource.Resource {
	return &TagResource{}
}

func (r *TagResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_tag"
}

func (r *TagResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Creates a Docker image tag. This resource allows you to create a new tag for an existing Docker image.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The ID of this resource (same as target_image).",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"source_image": schema.StringAttribute{
				Description: "The source Docker image name with tag (e.g., 'nginx:latest' or 'myregistry/myimage:v1.0').",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"target_image": schema.StringAttribute{
				Description: "The target Docker image name with tag (e.g., 'myregistry/nginx:v1.0').",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"source_image_id": schema.StringAttribute{
				Description: "The ID of the source image.",
				Computed:    true,
			},
		},
	}
}

func (r *TagResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *TagResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data TagResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	sourceImage := data.SourceImage.ValueString()
	targetImage := data.TargetImage.ValueString()

	// Inspect source image to get its ID
	srcInspect, _, err := r.client.ImageInspectWithRaw(ctx, sourceImage)
	if err != nil {
		resp.Diagnostics.AddError(
			"Source Image Not Found",
			fmt.Sprintf("Failed to find source image %s: %s", sourceImage, err),
		)
		return
	}

	tflog.Debug(ctx, "Creating Docker image tag", map[string]interface{}{
		"source": sourceImage,
		"target": targetImage,
	})

	// Create the tag
	err = r.client.ImageTag(ctx, sourceImage, targetImage)
	if err != nil {
		resp.Diagnostics.AddError(
			"Docker Tag Failed",
			fmt.Sprintf("Failed to tag image %s as %s: %s", sourceImage, targetImage, err),
		)
		return
	}

	data.ID = tftypes.StringValue(targetImage)
	data.SourceImageID = tftypes.StringValue(srcInspect.ID)

	tflog.Debug(ctx, "Created Docker image tag", map[string]interface{}{
		"id":              targetImage,
		"source_image_id": srcInspect.ID,
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *TagResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data TagResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	targetImage := data.TargetImage.ValueString()

	// Check if the target image exists
	inspect, _, err := r.client.ImageInspectWithRaw(ctx, targetImage)
	if err != nil {
		if strings.Contains(err.Error(), "No such image") {
			tflog.Debug(ctx, "Image tag not found, removing from state", map[string]interface{}{
				"target": targetImage,
			})
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Docker Image Read Failed",
			fmt.Sprintf("Failed to read image %s: %s", targetImage, err),
		)
		return
	}

	// Update source image ID if it changed (shouldn't happen but let's be safe)
	data.SourceImageID = tftypes.StringValue(inspect.ID)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *TagResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// Tags are immutable - any change requires replacement
	// This should not be called due to RequiresReplace plan modifiers
	var data TagResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *TagResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data TagResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	targetImage := data.TargetImage.ValueString()

	tflog.Debug(ctx, "Removing Docker image tag", map[string]interface{}{
		"target": targetImage,
	})

	// Remove the tag (not the underlying image)
	// We use ImageRemove with NoPrune to only remove the tag, not the image layers
	_, err := r.client.ImageRemove(ctx, targetImage, image.RemoveOptions{
		Force:         false,
		PruneChildren: false,
	})
	if err != nil {
		if strings.Contains(err.Error(), "No such image") {
			tflog.Debug(ctx, "Image tag already removed", map[string]interface{}{
				"target": targetImage,
			})
			return
		}
		resp.Diagnostics.AddError(
			"Docker Tag Deletion Failed",
			fmt.Sprintf("Failed to remove image tag %s: %s", targetImage, err),
		)
		return
	}

	tflog.Debug(ctx, "Removed Docker image tag", map[string]interface{}{
		"target": targetImage,
	})
}

func (r *TagResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Import by target image name
	targetImage := req.ID

	inspect, _, err := r.client.ImageInspectWithRaw(ctx, targetImage)
	if err != nil {
		resp.Diagnostics.AddError(
			"Import Failed",
			fmt.Sprintf("Failed to import image tag %s: %s", targetImage, err),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), targetImage)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("target_image"), targetImage)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("source_image_id"), inspect.ID)...)
	// Note: source_image cannot be determined from import - user must update config
	resp.Diagnostics.AddWarning(
		"Source Image Required",
		"The source_image attribute must be provided in the Terraform configuration after import.",
	)
}

