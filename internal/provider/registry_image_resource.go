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
	tftypes "github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var (
	_ resource.Resource                = &RegistryImageResource{}
	_ resource.ResourceWithImportState = &RegistryImageResource{}
)

type RegistryImageResource struct {
	client *docker.Client
}

type RegistryImageResourceModel struct {
	ID                 tftypes.String `tfsdk:"id"`
	Name               tftypes.String `tfsdk:"name"`
	KeepRemotely       tftypes.Bool   `tfsdk:"keep_remotely"`
	InsecureSkipVerify tftypes.Bool   `tfsdk:"insecure_skip_verify"`
	Triggers           tftypes.Map    `tfsdk:"triggers"`
	Sha256Digest       tftypes.String `tfsdk:"sha256_digest"`
	AuthConfig         tftypes.List   `tfsdk:"auth_config"`
	Build              tftypes.List   `tfsdk:"build"`
}

type RegistryAuthConfigModel struct {
	Address  tftypes.String `tfsdk:"address"`
	Username tftypes.String `tfsdk:"username"`
	Password tftypes.String `tfsdk:"password"`
}

type RegistryBuildModel struct {
	Context       tftypes.String `tfsdk:"context"`
	Dockerfile    tftypes.String `tfsdk:"dockerfile"`
	Target        tftypes.String `tfsdk:"target"`
	BuildArgs     tftypes.Map    `tfsdk:"build_args"`
	Labels        tftypes.Map    `tfsdk:"labels"`
	CacheFrom     tftypes.List   `tfsdk:"cache_from"`
	NoCache       tftypes.Bool   `tfsdk:"no_cache"`
	ForceRemove   tftypes.Bool   `tfsdk:"force_remove"`
	Platform      tftypes.String `tfsdk:"platform"`
}

func NewRegistryImageResource() resource.Resource {
	return &RegistryImageResource{}
}

func (r *RegistryImageResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_registry_image"
}

func (r *RegistryImageResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages the lifecycle of a Docker image in a registry. Pushes images to registries and can optionally build them first.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The ID of this resource.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "The full name of the Docker image including registry and tag (e.g., 'registry.example.com/myimage:v1.0').",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"keep_remotely": schema.BoolAttribute{
				Description: "If true, the image will not be deleted from the registry on destroy. Default is false.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"insecure_skip_verify": schema.BoolAttribute{
				Description: "If true, skip TLS certificate verification when pushing to the registry.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"triggers": schema.MapAttribute{
				Description: "A map of arbitrary values that, when changed, will cause the resource to be replaced.",
				Optional:    true,
				ElementType: tftypes.StringType,
			},
			"sha256_digest": schema.StringAttribute{
				Description: "The SHA256 digest of the pushed image.",
				Computed:    true,
			},
		},
		Blocks: map[string]schema.Block{
			"auth_config": schema.ListNestedBlock{
				Description: "Registry authentication configuration.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"address": schema.StringAttribute{
							Description: "Registry server address.",
							Required:    true,
						},
						"username": schema.StringAttribute{
							Description: "Registry username.",
							Required:    true,
						},
						"password": schema.StringAttribute{
							Description: "Registry password.",
							Required:    true,
							Sensitive:   true,
						},
					},
				},
			},
			"build": schema.ListNestedBlock{
				Description: "Optional build configuration. If provided, the image will be built before pushing.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"context": schema.StringAttribute{
							Description: "Build context path.",
							Required:    true,
						},
						"dockerfile": schema.StringAttribute{
							Description: "Dockerfile path relative to context. Default is 'Dockerfile'.",
							Optional:    true,
						},
						"target": schema.StringAttribute{
							Description: "Target build stage.",
							Optional:    true,
						},
						"build_args": schema.MapAttribute{
							Description: "Build arguments.",
							Optional:    true,
							ElementType: tftypes.StringType,
						},
						"labels": schema.MapAttribute{
							Description: "Image labels.",
							Optional:    true,
							ElementType: tftypes.StringType,
						},
						"cache_from": schema.ListAttribute{
							Description: "Images to use as cache sources.",
							Optional:    true,
							ElementType: tftypes.StringType,
						},
						"no_cache": schema.BoolAttribute{
							Description: "Do not use cache when building.",
							Optional:    true,
							Computed:    true,
							Default:     booldefault.StaticBool(false),
						},
						"force_remove": schema.BoolAttribute{
							Description: "Always remove intermediate containers.",
							Optional:    true,
							Computed:    true,
							Default:     booldefault.StaticBool(false),
						},
						"platform": schema.StringAttribute{
							Description: "Target platform (e.g., 'linux/amd64').",
							Optional:    true,
						},
					},
				},
			},
		},
	}
}

func (r *RegistryImageResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *RegistryImageResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data RegistryImageResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	imageName := data.Name.ValueString()

	// Get auth config
	encodedAuth := ""
	if !data.AuthConfig.IsNull() && len(data.AuthConfig.Elements()) > 0 {
		var authConfigs []RegistryAuthConfigModel
		resp.Diagnostics.Append(data.AuthConfig.ElementsAs(ctx, &authConfigs, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if len(authConfigs) > 0 {
			authConfig := registry.AuthConfig{
				ServerAddress: authConfigs[0].Address.ValueString(),
				Username:      authConfigs[0].Username.ValueString(),
				Password:      authConfigs[0].Password.ValueString(),
			}
			encodedJSON, _ := json.Marshal(authConfig)
			encodedAuth = base64.URLEncoding.EncodeToString(encodedJSON)
		}
	}

	// Build if configured
	if !data.Build.IsNull() && len(data.Build.Elements()) > 0 {
		tflog.Debug(ctx, "Building Docker image before push", map[string]interface{}{
			"name": imageName,
		})
		// Note: Build functionality would require tar archive creation and build API
		// For now, we assume the image already exists locally
		resp.Diagnostics.AddWarning(
			"Build Not Implemented",
			"Image building is not yet fully implemented. Please ensure the image exists locally before using this resource.",
		)
	}

	tflog.Debug(ctx, "Pushing Docker image to registry", map[string]interface{}{
		"name": imageName,
	})

	// Push the image
	pushReader, err := r.client.ImagePush(ctx, imageName, image.PushOptions{
		RegistryAuth: encodedAuth,
	})
	if err != nil {
		resp.Diagnostics.AddError(
			"Docker Image Push Failed",
			fmt.Sprintf("Failed to push image %s: %s", imageName, err),
		)
		return
	}
	defer pushReader.Close()

	// Read the push output to completion
	var digest string
	decoder := json.NewDecoder(pushReader)
	for {
		var pushResponse struct {
			Status         string `json:"status"`
			Progress       string `json:"progress"`
			ProgressDetail struct {
				Current int64 `json:"current"`
				Total   int64 `json:"total"`
			} `json:"progressDetail"`
			Aux struct {
				Digest string `json:"Digest"`
				Size   int64  `json:"Size"`
				Tag    string `json:"Tag"`
			} `json:"aux"`
			Error string `json:"error"`
		}

		if err := decoder.Decode(&pushResponse); err != nil {
			if err == io.EOF {
				break
			}
			// Continue on decode errors, not all lines are JSON
			continue
		}

		if pushResponse.Error != "" {
			resp.Diagnostics.AddError(
				"Docker Image Push Error",
				fmt.Sprintf("Error during push: %s", pushResponse.Error),
			)
			return
		}

		if pushResponse.Aux.Digest != "" {
			digest = pushResponse.Aux.Digest
		}
	}

	data.ID = tftypes.StringValue(imageName)
	if digest != "" {
		data.Sha256Digest = tftypes.StringValue(digest)
	} else {
		// Try to get digest from image inspect
		inspect, _, err := r.client.ImageInspectWithRaw(ctx, imageName)
		if err == nil && len(inspect.RepoDigests) > 0 {
			// Extract digest from repo digest (format: registry/image@sha256:xxx)
			for _, rd := range inspect.RepoDigests {
				if strings.Contains(rd, "@") {
					parts := strings.Split(rd, "@")
					if len(parts) == 2 {
						digest = parts[1]
						break
					}
				}
			}
		}
		if digest != "" {
			data.Sha256Digest = tftypes.StringValue(digest)
		} else {
			data.Sha256Digest = tftypes.StringNull()
		}
	}

	tflog.Debug(ctx, "Pushed Docker image to registry", map[string]interface{}{
		"name":   imageName,
		"digest": digest,
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *RegistryImageResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data RegistryImageResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	imageName := data.Name.ValueString()

	// Check if image exists locally
	inspect, _, err := r.client.ImageInspectWithRaw(ctx, imageName)
	if err != nil {
		if strings.Contains(err.Error(), "No such image") {
			// Image doesn't exist locally, but might still exist in registry
			// We can't easily verify registry existence without credentials
			tflog.Debug(ctx, "Image not found locally", map[string]interface{}{
				"name": imageName,
			})
			// Don't remove from state - the image might still exist in the registry
			// User should manually verify or re-push
			return
		}
		resp.Diagnostics.AddError(
			"Docker Image Read Failed",
			fmt.Sprintf("Failed to read image %s: %s", imageName, err),
		)
		return
	}

	// Update digest if available
	if len(inspect.RepoDigests) > 0 {
		for _, rd := range inspect.RepoDigests {
			if strings.Contains(rd, "@") {
				parts := strings.Split(rd, "@")
				if len(parts) == 2 {
					data.Sha256Digest = tftypes.StringValue(parts[1])
					break
				}
			}
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *RegistryImageResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data RegistryImageResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Check if triggers changed - if so, re-push the image
	var oldData RegistryImageResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &oldData)...)
	if resp.Diagnostics.HasError() {
		return
	}

	triggersChanged := !data.Triggers.Equal(oldData.Triggers)
	if triggersChanged {
		// Re-push the image
		imageName := data.Name.ValueString()

		encodedAuth := ""
		if !data.AuthConfig.IsNull() && len(data.AuthConfig.Elements()) > 0 {
			var authConfigs []RegistryAuthConfigModel
			resp.Diagnostics.Append(data.AuthConfig.ElementsAs(ctx, &authConfigs, false)...)
			if resp.Diagnostics.HasError() {
				return
			}
			if len(authConfigs) > 0 {
				authConfig := registry.AuthConfig{
					ServerAddress: authConfigs[0].Address.ValueString(),
					Username:      authConfigs[0].Username.ValueString(),
					Password:      authConfigs[0].Password.ValueString(),
				}
				encodedJSON, _ := json.Marshal(authConfig)
				encodedAuth = base64.URLEncoding.EncodeToString(encodedJSON)
			}
		}

		tflog.Debug(ctx, "Re-pushing Docker image due to trigger change", map[string]interface{}{
			"name": imageName,
		})

		pushReader, err := r.client.ImagePush(ctx, imageName, image.PushOptions{
			RegistryAuth: encodedAuth,
		})
		if err != nil {
			resp.Diagnostics.AddError(
				"Docker Image Push Failed",
				fmt.Sprintf("Failed to push image %s: %s", imageName, err),
			)
			return
		}
		defer pushReader.Close()

		// Read push output
		io.Copy(io.Discard, pushReader)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *RegistryImageResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data RegistryImageResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if data.KeepRemotely.ValueBool() {
		tflog.Debug(ctx, "Keeping image in registry (keep_remotely=true)", map[string]interface{}{
			"name": data.Name.ValueString(),
		})
		return
	}

	// Note: Docker API doesn't provide a direct way to delete images from registries
	// Registry deletion would require registry-specific API calls
	// For now, we just log a warning
	tflog.Warn(ctx, "Registry image deletion not implemented - image remains in registry", map[string]interface{}{
		"name": data.Name.ValueString(),
	})

	resp.Diagnostics.AddWarning(
		"Registry Deletion Not Supported",
		fmt.Sprintf("The Docker API does not support deleting images from registries. Image %s may still exist in the registry. Use keep_remotely=true to suppress this warning.", data.Name.ValueString()),
	)
}

func (r *RegistryImageResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Import by image name
	imageName := req.ID

	inspect, _, err := r.client.ImageInspectWithRaw(ctx, imageName)
	if err != nil {
		resp.Diagnostics.AddError(
			"Import Failed",
			fmt.Sprintf("Failed to import registry image %s: %s", imageName, err),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), imageName)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), imageName)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("keep_remotely"), false)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("insecure_skip_verify"), false)...)

	// Try to get digest
	if len(inspect.RepoDigests) > 0 {
		for _, rd := range inspect.RepoDigests {
			if strings.Contains(rd, "@") {
				parts := strings.Split(rd, "@")
				if len(parts) == 2 {
					resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("sha256_digest"), parts[1])...)
					break
				}
			}
		}
	}
}

