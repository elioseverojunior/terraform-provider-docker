package provider

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/registry"
	"github.com/elioseverojunior/terraform-provider-docker/internal/docker"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var (
	_ resource.Resource                = &ImageResource{}
	_ resource.ResourceWithImportState = &ImageResource{}
)

type ImageResource struct {
	client *docker.Client
}

type ImageResourceModel struct {
	ID           types.String `tfsdk:"id"`
	Name         types.String `tfsdk:"name"`
	KeepLocally  types.Bool   `tfsdk:"keep_locally"`
	ForceRemove  types.Bool   `tfsdk:"force_remove"`
	PullTriggers types.List   `tfsdk:"pull_triggers"`
	ImageID      types.String `tfsdk:"image_id"`
	RepoDigest   types.String `tfsdk:"repo_digest"`

	// Registry authentication
	RegistryAuth *RegistryAuthModel `tfsdk:"registry_auth"`
}

type RegistryAuthModel struct {
	Address  types.String `tfsdk:"address"`
	Username types.String `tfsdk:"username"`
	Password types.String `tfsdk:"password"`
}

func NewImageResource() resource.Resource {
	return &ImageResource{}
}

func (r *ImageResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_image"
}

func (r *ImageResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages Docker images. Pulls images from a registry and optionally keeps them locally.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The ID of this resource.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "The name of the Docker image, including any tags or SHA256 repo digests (e.g., nginx:latest, nginx@sha256:...).",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"keep_locally": schema.BoolAttribute{
				Description: "If true, the image won't be deleted on destroy operation. Default is false.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"force_remove": schema.BoolAttribute{
				Description: "If true, forces the removal of the image even if it's being used by stopped containers.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"pull_triggers": schema.ListAttribute{
				Description: "List of values which cause an image pull when changed.",
				Optional:    true,
				ElementType: types.StringType,
			},
			"image_id": schema.StringAttribute{
				Description: "The ID of the pulled image.",
				Computed:    true,
			},
			"repo_digest": schema.StringAttribute{
				Description: "The image digest in the form of repo@sha256:...",
				Computed:    true,
			},
		},
		Blocks: map[string]schema.Block{
			"registry_auth": schema.SingleNestedBlock{
				Description: "Registry authentication configuration.",
				Attributes: map[string]schema.Attribute{
					"address": schema.StringAttribute{
						Description: "The address of the registry (e.g., docker.io, gcr.io).",
						Optional:    true,
					},
					"username": schema.StringAttribute{
						Description: "The username for registry authentication.",
						Optional:    true,
					},
					"password": schema.StringAttribute{
						Description: "The password for registry authentication.",
						Optional:    true,
						Sensitive:   true,
					},
				},
			},
		},
	}
}

func (r *ImageResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *ImageResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data ImageResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	imageName := data.Name.ValueString()
	tflog.Debug(ctx, "Pulling Docker image", map[string]interface{}{
		"name": imageName,
	})

	pullOptions := image.PullOptions{}

	// Configure registry authentication if provided
	if data.RegistryAuth != nil && !data.RegistryAuth.Username.IsNull() {
		authConfig := registry.AuthConfig{
			Username: data.RegistryAuth.Username.ValueString(),
			Password: data.RegistryAuth.Password.ValueString(),
		}
		if !data.RegistryAuth.Address.IsNull() {
			authConfig.ServerAddress = data.RegistryAuth.Address.ValueString()
		}

		encodedJSON, err := json.Marshal(authConfig)
		if err != nil {
			resp.Diagnostics.AddError("Registry Auth Error", fmt.Sprintf("Failed to encode registry auth: %s", err))
			return
		}
		pullOptions.RegistryAuth = base64.URLEncoding.EncodeToString(encodedJSON)
	}

	// Pull the image
	reader, err := r.client.ImagePull(ctx, imageName, pullOptions)
	if err != nil {
		resp.Diagnostics.AddError("Image Pull Error", fmt.Sprintf("Unable to pull image %s: %s", imageName, err))
		return
	}
	defer reader.Close()

	// Consume the output to complete the pull
	_, err = io.Copy(io.Discard, reader)
	if err != nil {
		resp.Diagnostics.AddError("Image Pull Error", fmt.Sprintf("Error during image pull %s: %s", imageName, err))
		return
	}

	// Inspect the pulled image to get its ID and digest
	imageInspect, _, err := r.client.ImageInspectWithRaw(ctx, imageName)
	if err != nil {
		resp.Diagnostics.AddError("Image Inspect Error", fmt.Sprintf("Unable to inspect pulled image %s: %s", imageName, err))
		return
	}

	data.ID = types.StringValue(imageInspect.ID)
	data.ImageID = types.StringValue(imageInspect.ID)

	// Get repo digest if available
	if len(imageInspect.RepoDigests) > 0 {
		data.RepoDigest = types.StringValue(imageInspect.RepoDigests[0])
	} else {
		data.RepoDigest = types.StringNull()
	}

	tflog.Debug(ctx, "Created Docker image resource", map[string]interface{}{
		"name":     imageName,
		"image_id": imageInspect.ID,
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ImageResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data ImageResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	imageName := data.Name.ValueString()

	imageInspect, _, err := r.client.ImageInspectWithRaw(ctx, imageName)
	if err != nil {
		if strings.Contains(err.Error(), "No such image") {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Image Read Error", fmt.Sprintf("Unable to read image %s: %s", imageName, err))
		return
	}

	data.ImageID = types.StringValue(imageInspect.ID)

	if len(imageInspect.RepoDigests) > 0 {
		data.RepoDigest = types.StringValue(imageInspect.RepoDigests[0])
	} else {
		data.RepoDigest = types.StringNull()
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ImageResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data ImageResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Re-pull the image if pull_triggers changed
	imageName := data.Name.ValueString()

	pullOptions := image.PullOptions{}

	if data.RegistryAuth != nil && !data.RegistryAuth.Username.IsNull() {
		authConfig := registry.AuthConfig{
			Username: data.RegistryAuth.Username.ValueString(),
			Password: data.RegistryAuth.Password.ValueString(),
		}
		if !data.RegistryAuth.Address.IsNull() {
			authConfig.ServerAddress = data.RegistryAuth.Address.ValueString()
		}

		encodedJSON, err := json.Marshal(authConfig)
		if err != nil {
			resp.Diagnostics.AddError("Registry Auth Error", fmt.Sprintf("Failed to encode registry auth: %s", err))
			return
		}
		pullOptions.RegistryAuth = base64.URLEncoding.EncodeToString(encodedJSON)
	}

	reader, err := r.client.ImagePull(ctx, imageName, pullOptions)
	if err != nil {
		resp.Diagnostics.AddError("Image Pull Error", fmt.Sprintf("Unable to pull image %s: %s", imageName, err))
		return
	}
	defer reader.Close()

	_, err = io.Copy(io.Discard, reader)
	if err != nil {
		resp.Diagnostics.AddError("Image Pull Error", fmt.Sprintf("Error during image pull %s: %s", imageName, err))
		return
	}

	imageInspect, _, err := r.client.ImageInspectWithRaw(ctx, imageName)
	if err != nil {
		resp.Diagnostics.AddError("Image Inspect Error", fmt.Sprintf("Unable to inspect pulled image %s: %s", imageName, err))
		return
	}

	data.ID = types.StringValue(imageInspect.ID)
	data.ImageID = types.StringValue(imageInspect.ID)

	if len(imageInspect.RepoDigests) > 0 {
		data.RepoDigest = types.StringValue(imageInspect.RepoDigests[0])
	} else {
		data.RepoDigest = types.StringNull()
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ImageResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data ImageResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if data.KeepLocally.ValueBool() {
		tflog.Debug(ctx, "Keeping image locally as configured", map[string]interface{}{
			"name": data.Name.ValueString(),
		})
		return
	}

	imageName := data.Name.ValueString()

	removeOptions := image.RemoveOptions{
		Force:         data.ForceRemove.ValueBool(),
		PruneChildren: true,
	}

	_, err := r.client.ImageRemove(ctx, imageName, removeOptions)
	if err != nil {
		if strings.Contains(err.Error(), "No such image") {
			return
		}
		resp.Diagnostics.AddError("Image Delete Error", fmt.Sprintf("Unable to remove image %s: %s", imageName, err))
		return
	}

	tflog.Debug(ctx, "Deleted Docker image", map[string]interface{}{
		"name": imageName,
	})
}

func (r *ImageResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("name"), req, resp)
}
