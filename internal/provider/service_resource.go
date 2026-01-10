package provider

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/registry"
	"github.com/docker/docker/api/types/swarm"
	"github.com/elioseverojunior/terraform-provider-docker/internal/docker"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	tftypes "github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var (
	_ resource.Resource                = &ServiceResource{}
	_ resource.ResourceWithImportState = &ServiceResource{}
)

type ServiceResource struct {
	client *docker.Client
}

type ServiceResourceModel struct {
	ID             tftypes.String `tfsdk:"id"`
	Name           tftypes.String `tfsdk:"name"`
	Labels         tftypes.Map    `tfsdk:"labels"`
	TaskSpec       tftypes.List   `tfsdk:"task_spec"`
	Mode           tftypes.String `tfsdk:"mode"`
	Replicas       tftypes.Int64  `tfsdk:"replicas"`
	EndpointSpec   tftypes.List   `tfsdk:"endpoint_spec"`
	UpdateConfig   tftypes.List   `tfsdk:"update_config"`
	RollbackConfig tftypes.List   `tfsdk:"rollback_config"`
	ConvergeConfig tftypes.List   `tfsdk:"converge_config"`
	Auth           tftypes.List   `tfsdk:"auth"`
}

type TaskSpecModel struct {
	ContainerSpec tftypes.List  `tfsdk:"container_spec"`
	Resources     tftypes.List  `tfsdk:"resources"`
	RestartPolicy tftypes.List  `tfsdk:"restart_policy"`
	Placement     tftypes.List  `tfsdk:"placement"`
	Networks      tftypes.Set   `tfsdk:"networks"`
	LogDriver     tftypes.List  `tfsdk:"log_driver"`
	ForceUpdate   tftypes.Int64 `tfsdk:"force_update"`
}

type ContainerSpecModel struct {
	Image           tftypes.String `tfsdk:"image"`
	Command         tftypes.List   `tfsdk:"command"`
	Args            tftypes.List   `tfsdk:"args"`
	Hostname        tftypes.String `tfsdk:"hostname"`
	Env             tftypes.Map    `tfsdk:"env"`
	Dir             tftypes.String `tfsdk:"dir"`
	User            tftypes.String `tfsdk:"user"`
	Groups          tftypes.List   `tfsdk:"groups"`
	Privileges      tftypes.List   `tfsdk:"privileges"`
	ReadOnly        tftypes.Bool   `tfsdk:"read_only"`
	Mounts          tftypes.List   `tfsdk:"mounts"`
	StopSignal      tftypes.String `tfsdk:"stop_signal"`
	StopGracePeriod tftypes.String `tfsdk:"stop_grace_period"`
	Healthcheck     tftypes.List   `tfsdk:"healthcheck"`
	Hosts           tftypes.List   `tfsdk:"hosts"`
	DNSConfig       tftypes.List   `tfsdk:"dns_config"`
	Secrets         tftypes.List   `tfsdk:"secrets"`
	Configs         tftypes.List   `tfsdk:"configs"`
	Labels          tftypes.Map    `tfsdk:"labels"`
}

type MountModel struct {
	Target        tftypes.String `tfsdk:"target"`
	Source        tftypes.String `tfsdk:"source"`
	Type          tftypes.String `tfsdk:"type"`
	ReadOnly      tftypes.Bool   `tfsdk:"read_only"`
	BindOptions   tftypes.List   `tfsdk:"bind_options"`
	VolumeOptions tftypes.List   `tfsdk:"volume_options"`
	TmpfsOptions  tftypes.List   `tfsdk:"tmpfs_options"`
}

type EndpointSpecModel struct {
	Mode  tftypes.String `tfsdk:"mode"`
	Ports tftypes.List   `tfsdk:"ports"`
}

type PortConfigModel struct {
	Name          tftypes.String `tfsdk:"name"`
	Protocol      tftypes.String `tfsdk:"protocol"`
	TargetPort    tftypes.Int64  `tfsdk:"target_port"`
	PublishedPort tftypes.Int64  `tfsdk:"published_port"`
	PublishMode   tftypes.String `tfsdk:"publish_mode"`
}

type UpdateConfigModel struct {
	Parallelism     tftypes.Int64   `tfsdk:"parallelism"`
	Delay           tftypes.String  `tfsdk:"delay"`
	FailureAction   tftypes.String  `tfsdk:"failure_action"`
	Monitor         tftypes.String  `tfsdk:"monitor"`
	MaxFailureRatio tftypes.Float64 `tfsdk:"max_failure_ratio"`
	Order           tftypes.String  `tfsdk:"order"`
}

type ConvergeConfigModel struct {
	Delay   tftypes.String `tfsdk:"delay"`
	Timeout tftypes.String `tfsdk:"timeout"`
}

type AuthModel struct {
	ServerAddress tftypes.String `tfsdk:"server_address"`
	Username      tftypes.String `tfsdk:"username"`
	Password      tftypes.String `tfsdk:"password"`
}

func NewServiceResource() resource.Resource {
	return &ServiceResource{}
}

func (r *ServiceResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_service"
}

func (r *ServiceResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages Docker Swarm services. Requires Docker to be in Swarm mode.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The ID of this resource.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "The name of the Docker service.",
				Required:    true,
			},
			"labels": schema.MapAttribute{
				Description: "User-defined key/value metadata for the service.",
				Optional:    true,
				ElementType: tftypes.StringType,
			},
			"mode": schema.StringAttribute{
				Description: "Service mode: 'replicated' or 'global'. Default is 'replicated'.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString("replicated"),
			},
			"replicas": schema.Int64Attribute{
				Description: "Number of replicas for the service. Only applicable in 'replicated' mode.",
				Optional:    true,
				Computed:    true,
				Default:     int64default.StaticInt64(1),
			},
		},
		Blocks: map[string]schema.Block{
			"task_spec": schema.ListNestedBlock{
				Description: "Task specification for the service.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"networks": schema.SetAttribute{
							Description: "Networks to attach the container to.",
							Optional:    true,
							ElementType: tftypes.StringType,
						},
						"force_update": schema.Int64Attribute{
							Description: "Counter to trigger a forced update.",
							Optional:    true,
							Computed:    true,
							Default:     int64default.StaticInt64(0),
						},
					},
					Blocks: map[string]schema.Block{
						"container_spec": schema.ListNestedBlock{
							Description: "Container specification.",
							NestedObject: schema.NestedBlockObject{
								Attributes: map[string]schema.Attribute{
									"image": schema.StringAttribute{
										Description: "The image to use for the container.",
										Required:    true,
									},
									"hostname": schema.StringAttribute{
										Description: "Container hostname.",
										Optional:    true,
									},
									"env": schema.MapAttribute{
										Description: "Environment variables.",
										Optional:    true,
										ElementType: tftypes.StringType,
									},
									"dir": schema.StringAttribute{
										Description: "Working directory.",
										Optional:    true,
									},
									"user": schema.StringAttribute{
										Description: "User to run the container as.",
										Optional:    true,
									},
									"read_only": schema.BoolAttribute{
										Description: "Mount the container's root filesystem as read-only.",
										Optional:    true,
										Computed:    true,
										Default:     booldefault.StaticBool(false),
									},
									"stop_signal": schema.StringAttribute{
										Description: "Signal to stop the container.",
										Optional:    true,
									},
									"stop_grace_period": schema.StringAttribute{
										Description: "Time to wait before forcefully killing the container.",
										Optional:    true,
									},
									"labels": schema.MapAttribute{
										Description: "Container labels.",
										Optional:    true,
										ElementType: tftypes.StringType,
									},
								},
								Blocks: map[string]schema.Block{
									"command": schema.ListNestedBlock{
										Description: "Command to run in the container.",
										NestedObject: schema.NestedBlockObject{
											Attributes: map[string]schema.Attribute{
												"value": schema.StringAttribute{
													Description: "Command argument.",
													Required:    true,
												},
											},
										},
									},
									"args": schema.ListNestedBlock{
										Description: "Arguments to the command.",
										NestedObject: schema.NestedBlockObject{
											Attributes: map[string]schema.Attribute{
												"value": schema.StringAttribute{
													Description: "Argument value.",
													Required:    true,
												},
											},
										},
									},
									"groups": schema.ListNestedBlock{
										Description: "Additional groups.",
										NestedObject: schema.NestedBlockObject{
											Attributes: map[string]schema.Attribute{
												"value": schema.StringAttribute{
													Description: "Group name.",
													Required:    true,
												},
											},
										},
									},
									"mounts": schema.ListNestedBlock{
										Description: "Mount configurations.",
										NestedObject: schema.NestedBlockObject{
											Attributes: map[string]schema.Attribute{
												"target": schema.StringAttribute{
													Description: "Mount target path in the container.",
													Required:    true,
												},
												"source": schema.StringAttribute{
													Description: "Mount source (volume name or host path).",
													Optional:    true,
												},
												"type": schema.StringAttribute{
													Description: "Mount type: bind, volume, or tmpfs.",
													Required:    true,
												},
												"read_only": schema.BoolAttribute{
													Description: "Mount as read-only.",
													Optional:    true,
													Computed:    true,
													Default:     booldefault.StaticBool(false),
												},
											},
										},
									},
									"hosts": schema.ListNestedBlock{
										Description: "Extra hosts to add.",
										NestedObject: schema.NestedBlockObject{
											Attributes: map[string]schema.Attribute{
												"host": schema.StringAttribute{
													Description: "Hostname.",
													Required:    true,
												},
												"ip": schema.StringAttribute{
													Description: "IP address.",
													Required:    true,
												},
											},
										},
									},
									"healthcheck": schema.ListNestedBlock{
										Description: "Health check configuration.",
										NestedObject: schema.NestedBlockObject{
											Attributes: map[string]schema.Attribute{
												"test": schema.ListAttribute{
													Description: "Health check test command.",
													Required:    true,
													ElementType: tftypes.StringType,
												},
												"interval": schema.StringAttribute{
													Description: "Interval between health checks.",
													Optional:    true,
													Computed:    true,
													Default:     stringdefault.StaticString("30s"),
												},
												"timeout": schema.StringAttribute{
													Description: "Health check timeout.",
													Optional:    true,
													Computed:    true,
													Default:     stringdefault.StaticString("30s"),
												},
												"start_period": schema.StringAttribute{
													Description: "Start period for the container to initialize.",
													Optional:    true,
													Computed:    true,
													Default:     stringdefault.StaticString("0s"),
												},
												"retries": schema.Int64Attribute{
													Description: "Number of retries before marking unhealthy.",
													Optional:    true,
													Computed:    true,
													Default:     int64default.StaticInt64(3),
												},
											},
										},
									},
									"secrets": schema.ListNestedBlock{
										Description: "Secrets to expose to the container.",
										NestedObject: schema.NestedBlockObject{
											Attributes: map[string]schema.Attribute{
												"secret_id": schema.StringAttribute{
													Description: "Secret ID.",
													Required:    true,
												},
												"secret_name": schema.StringAttribute{
													Description: "Secret name.",
													Required:    true,
												},
												"file_name": schema.StringAttribute{
													Description: "Target file name in the container.",
													Required:    true,
												},
												"file_uid": schema.StringAttribute{
													Description: "UID of the secret file.",
													Optional:    true,
													Computed:    true,
													Default:     stringdefault.StaticString("0"),
												},
												"file_gid": schema.StringAttribute{
													Description: "GID of the secret file.",
													Optional:    true,
													Computed:    true,
													Default:     stringdefault.StaticString("0"),
												},
												"file_mode": schema.Int64Attribute{
													Description: "File mode of the secret file.",
													Optional:    true,
													Computed:    true,
													Default:     int64default.StaticInt64(0444),
												},
											},
										},
									},
									"configs": schema.ListNestedBlock{
										Description: "Configs to expose to the container.",
										NestedObject: schema.NestedBlockObject{
											Attributes: map[string]schema.Attribute{
												"config_id": schema.StringAttribute{
													Description: "Config ID.",
													Required:    true,
												},
												"config_name": schema.StringAttribute{
													Description: "Config name.",
													Required:    true,
												},
												"file_name": schema.StringAttribute{
													Description: "Target file name in the container.",
													Required:    true,
												},
												"file_uid": schema.StringAttribute{
													Description: "UID of the config file.",
													Optional:    true,
													Computed:    true,
													Default:     stringdefault.StaticString("0"),
												},
												"file_gid": schema.StringAttribute{
													Description: "GID of the config file.",
													Optional:    true,
													Computed:    true,
													Default:     stringdefault.StaticString("0"),
												},
												"file_mode": schema.Int64Attribute{
													Description: "File mode of the config file.",
													Optional:    true,
													Computed:    true,
													Default:     int64default.StaticInt64(0444),
												},
											},
										},
									},
									"dns_config": schema.ListNestedBlock{
										Description: "DNS configuration.",
										NestedObject: schema.NestedBlockObject{
											Attributes: map[string]schema.Attribute{
												"nameservers": schema.ListAttribute{
													Description: "DNS nameservers.",
													Optional:    true,
													ElementType: tftypes.StringType,
												},
												"search": schema.ListAttribute{
													Description: "DNS search domains.",
													Optional:    true,
													ElementType: tftypes.StringType,
												},
												"options": schema.ListAttribute{
													Description: "DNS options.",
													Optional:    true,
													ElementType: tftypes.StringType,
												},
											},
										},
									},
									"privileges": schema.ListNestedBlock{
										Description: "Privilege configuration.",
										NestedObject: schema.NestedBlockObject{
											Attributes: map[string]schema.Attribute{
												"no_new_privileges": schema.BoolAttribute{
													Description: "Disable gaining additional privileges.",
													Optional:    true,
												},
											},
										},
									},
								},
							},
						},
						"resources": schema.ListNestedBlock{
							Description: "Resource limits and reservations.",
							NestedObject: schema.NestedBlockObject{
								Blocks: map[string]schema.Block{
									"limits": schema.ListNestedBlock{
										Description: "Resource limits.",
										NestedObject: schema.NestedBlockObject{
											Attributes: map[string]schema.Attribute{
												"nano_cpus": schema.Int64Attribute{
													Description: "CPU limit in nano CPUs.",
													Optional:    true,
												},
												"memory_bytes": schema.Int64Attribute{
													Description: "Memory limit in bytes.",
													Optional:    true,
												},
											},
										},
									},
									"reservations": schema.ListNestedBlock{
										Description: "Resource reservations.",
										NestedObject: schema.NestedBlockObject{
											Attributes: map[string]schema.Attribute{
												"nano_cpus": schema.Int64Attribute{
													Description: "CPU reservation in nano CPUs.",
													Optional:    true,
												},
												"memory_bytes": schema.Int64Attribute{
													Description: "Memory reservation in bytes.",
													Optional:    true,
												},
											},
										},
									},
								},
							},
						},
						"restart_policy": schema.ListNestedBlock{
							Description: "Restart policy.",
							NestedObject: schema.NestedBlockObject{
								Attributes: map[string]schema.Attribute{
									"condition": schema.StringAttribute{
										Description: "Restart condition: none, on-failure, any.",
										Optional:    true,
										Computed:    true,
										Default:     stringdefault.StaticString("any"),
									},
									"delay": schema.StringAttribute{
										Description: "Delay between restart attempts.",
										Optional:    true,
										Computed:    true,
										Default:     stringdefault.StaticString("5s"),
									},
									"max_attempts": schema.Int64Attribute{
										Description: "Maximum restart attempts.",
										Optional:    true,
									},
									"window": schema.StringAttribute{
										Description: "Time window to evaluate restart policy.",
										Optional:    true,
									},
								},
							},
						},
						"placement": schema.ListNestedBlock{
							Description: "Placement constraints and preferences.",
							NestedObject: schema.NestedBlockObject{
								Attributes: map[string]schema.Attribute{
									"constraints": schema.SetAttribute{
										Description: "Placement constraints.",
										Optional:    true,
										ElementType: tftypes.StringType,
									},
									"prefs": schema.SetAttribute{
										Description: "Placement preferences.",
										Optional:    true,
										ElementType: tftypes.StringType,
									},
									"max_replicas": schema.Int64Attribute{
										Description: "Maximum replicas per node.",
										Optional:    true,
									},
								},
							},
						},
						"log_driver": schema.ListNestedBlock{
							Description: "Log driver configuration.",
							NestedObject: schema.NestedBlockObject{
								Attributes: map[string]schema.Attribute{
									"name": schema.StringAttribute{
										Description: "Log driver name.",
										Required:    true,
									},
									"options": schema.MapAttribute{
										Description: "Log driver options.",
										Optional:    true,
										ElementType: tftypes.StringType,
									},
								},
							},
						},
					},
				},
			},
			"endpoint_spec": schema.ListNestedBlock{
				Description: "Endpoint specification.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"mode": schema.StringAttribute{
							Description: "Endpoint mode: vip or dnsrr.",
							Optional:    true,
							Computed:    true,
							Default:     stringdefault.StaticString("vip"),
						},
					},
					Blocks: map[string]schema.Block{
						"ports": schema.ListNestedBlock{
							Description: "Port configurations.",
							NestedObject: schema.NestedBlockObject{
								Attributes: map[string]schema.Attribute{
									"name": schema.StringAttribute{
										Description: "Port name.",
										Optional:    true,
									},
									"protocol": schema.StringAttribute{
										Description: "Protocol: tcp, udp, or sctp.",
										Optional:    true,
										Computed:    true,
										Default:     stringdefault.StaticString("tcp"),
									},
									"target_port": schema.Int64Attribute{
										Description: "Target port inside the container.",
										Required:    true,
									},
									"published_port": schema.Int64Attribute{
										Description: "Published port on the swarm.",
										Optional:    true,
									},
									"publish_mode": schema.StringAttribute{
										Description: "Port publish mode: ingress or host.",
										Optional:    true,
										Computed:    true,
										Default:     stringdefault.StaticString("ingress"),
									},
								},
							},
						},
					},
				},
			},
			"update_config": schema.ListNestedBlock{
				Description: "Update configuration.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"parallelism": schema.Int64Attribute{
							Description: "Number of tasks to update simultaneously.",
							Optional:    true,
							Computed:    true,
							Default:     int64default.StaticInt64(1),
						},
						"delay": schema.StringAttribute{
							Description: "Delay between updates.",
							Optional:    true,
							Computed:    true,
							Default:     stringdefault.StaticString("0s"),
						},
						"failure_action": schema.StringAttribute{
							Description: "Action on update failure: continue, pause, or rollback.",
							Optional:    true,
							Computed:    true,
							Default:     stringdefault.StaticString("pause"),
						},
						"monitor": schema.StringAttribute{
							Description: "Duration to monitor after update.",
							Optional:    true,
							Computed:    true,
							Default:     stringdefault.StaticString("5s"),
						},
						"max_failure_ratio": schema.Float64Attribute{
							Description: "Maximum failure ratio.",
							Optional:    true,
						},
						"order": schema.StringAttribute{
							Description: "Update order: stop-first or start-first.",
							Optional:    true,
							Computed:    true,
							Default:     stringdefault.StaticString("stop-first"),
						},
					},
				},
			},
			"rollback_config": schema.ListNestedBlock{
				Description: "Rollback configuration.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"parallelism": schema.Int64Attribute{
							Description: "Number of tasks to rollback simultaneously.",
							Optional:    true,
							Computed:    true,
							Default:     int64default.StaticInt64(1),
						},
						"delay": schema.StringAttribute{
							Description: "Delay between rollbacks.",
							Optional:    true,
							Computed:    true,
							Default:     stringdefault.StaticString("0s"),
						},
						"failure_action": schema.StringAttribute{
							Description: "Action on rollback failure: continue or pause.",
							Optional:    true,
							Computed:    true,
							Default:     stringdefault.StaticString("pause"),
						},
						"monitor": schema.StringAttribute{
							Description: "Duration to monitor after rollback.",
							Optional:    true,
							Computed:    true,
							Default:     stringdefault.StaticString("5s"),
						},
						"max_failure_ratio": schema.Float64Attribute{
							Description: "Maximum failure ratio.",
							Optional:    true,
						},
						"order": schema.StringAttribute{
							Description: "Rollback order: stop-first or start-first.",
							Optional:    true,
							Computed:    true,
							Default:     stringdefault.StaticString("stop-first"),
						},
					},
				},
			},
			"converge_config": schema.ListNestedBlock{
				Description: "Converge configuration for synchronous operations.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"delay": schema.StringAttribute{
							Description: "Delay between convergence checks.",
							Optional:    true,
							Computed:    true,
							Default:     stringdefault.StaticString("7s"),
						},
						"timeout": schema.StringAttribute{
							Description: "Timeout for convergence.",
							Optional:    true,
							Computed:    true,
							Default:     stringdefault.StaticString("3m"),
						},
					},
				},
			},
			"auth": schema.ListNestedBlock{
				Description: "Registry authentication for private images.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"server_address": schema.StringAttribute{
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
		},
	}
}

func (r *ServiceResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *ServiceResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data ServiceResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	serviceSpec, err := r.buildServiceSpec(ctx, &data, &resp.Diagnostics)
	if err != nil {
		resp.Diagnostics.AddError("Failed to build service spec", err.Error())
		return
	}

	// Build auth config
	var encodedAuth string
	if !data.Auth.IsNull() && len(data.Auth.Elements()) > 0 {
		var authModels []struct {
			ServerAddress tftypes.String `tfsdk:"server_address"`
			Username      tftypes.String `tfsdk:"username"`
			Password      tftypes.String `tfsdk:"password"`
		}
		resp.Diagnostics.Append(data.Auth.ElementsAs(ctx, &authModels, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if len(authModels) > 0 {
			authConfig := registry.AuthConfig{
				ServerAddress: authModels[0].ServerAddress.ValueString(),
				Username:      authModels[0].Username.ValueString(),
				Password:      authModels[0].Password.ValueString(),
			}
			encodedJSON, _ := json.Marshal(authConfig)
			encodedAuth = base64.URLEncoding.EncodeToString(encodedJSON)
		}
	}

	tflog.Debug(ctx, "Creating Docker service", map[string]interface{}{
		"name": data.Name.ValueString(),
	})

	serviceCreateResponse, err := r.client.ServiceCreate(ctx, *serviceSpec, types.ServiceCreateOptions{
		EncodedRegistryAuth: encodedAuth,
	})
	if err != nil {
		resp.Diagnostics.AddError(
			"Docker Service Creation Failed",
			fmt.Sprintf("Failed to create service %s: %s. Note: Docker must be in Swarm mode.", data.Name.ValueString(), err),
		)
		return
	}

	data.ID = tftypes.StringValue(serviceCreateResponse.ID)

	// Wait for convergence if configured
	if !data.ConvergeConfig.IsNull() && len(data.ConvergeConfig.Elements()) > 0 {
		r.waitForConvergence(ctx, data.ID.ValueString(), data.ConvergeConfig, &resp.Diagnostics)
	}

	tflog.Debug(ctx, "Created Docker service", map[string]interface{}{
		"id":   serviceCreateResponse.ID,
		"name": data.Name.ValueString(),
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ServiceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data ServiceResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	service, _, err := r.client.ServiceInspectWithRaw(ctx, data.ID.ValueString(), types.ServiceInspectOptions{})
	if err != nil {
		if strings.Contains(err.Error(), "No such service") || strings.Contains(err.Error(), "service not found") {
			tflog.Debug(ctx, "Service not found, removing from state", map[string]interface{}{
				"id": data.ID.ValueString(),
			})
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Docker Service Read Failed",
			fmt.Sprintf("Failed to read service %s: %s", data.ID.ValueString(), err),
		)
		return
	}

	data.Name = tftypes.StringValue(service.Spec.Name)

	if len(service.Spec.Labels) > 0 {
		labels, diags := tftypes.MapValueFrom(ctx, tftypes.StringType, service.Spec.Labels)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		data.Labels = labels
	}

	// Set mode
	if service.Spec.Mode.Global != nil {
		data.Mode = tftypes.StringValue("global")
	} else {
		data.Mode = tftypes.StringValue("replicated")
		if service.Spec.Mode.Replicated != nil && service.Spec.Mode.Replicated.Replicas != nil {
			data.Replicas = tftypes.Int64Value(int64(*service.Spec.Mode.Replicated.Replicas))
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ServiceResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data ServiceResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Get current service to get version
	service, _, err := r.client.ServiceInspectWithRaw(ctx, data.ID.ValueString(), types.ServiceInspectOptions{})
	if err != nil {
		resp.Diagnostics.AddError(
			"Docker Service Update Failed",
			fmt.Sprintf("Failed to inspect service %s: %s", data.ID.ValueString(), err),
		)
		return
	}

	serviceSpec, err := r.buildServiceSpec(ctx, &data, &resp.Diagnostics)
	if err != nil {
		resp.Diagnostics.AddError("Failed to build service spec", err.Error())
		return
	}

	// Build auth config
	var encodedAuth string
	if !data.Auth.IsNull() && len(data.Auth.Elements()) > 0 {
		var authModels []struct {
			ServerAddress tftypes.String `tfsdk:"server_address"`
			Username      tftypes.String `tfsdk:"username"`
			Password      tftypes.String `tfsdk:"password"`
		}
		resp.Diagnostics.Append(data.Auth.ElementsAs(ctx, &authModels, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if len(authModels) > 0 {
			authConfig := registry.AuthConfig{
				ServerAddress: authModels[0].ServerAddress.ValueString(),
				Username:      authModels[0].Username.ValueString(),
				Password:      authModels[0].Password.ValueString(),
			}
			encodedJSON, _ := json.Marshal(authConfig)
			encodedAuth = base64.URLEncoding.EncodeToString(encodedJSON)
		}
	}

	tflog.Debug(ctx, "Updating Docker service", map[string]interface{}{
		"id":   data.ID.ValueString(),
		"name": data.Name.ValueString(),
	})

	_, err = r.client.ServiceUpdate(ctx, data.ID.ValueString(), service.Version, *serviceSpec, types.ServiceUpdateOptions{
		EncodedRegistryAuth: encodedAuth,
	})
	if err != nil {
		resp.Diagnostics.AddError(
			"Docker Service Update Failed",
			fmt.Sprintf("Failed to update service %s: %s", data.ID.ValueString(), err),
		)
		return
	}

	// Wait for convergence if configured
	if !data.ConvergeConfig.IsNull() && len(data.ConvergeConfig.Elements()) > 0 {
		r.waitForConvergence(ctx, data.ID.ValueString(), data.ConvergeConfig, &resp.Diagnostics)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ServiceResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data ServiceResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Deleting Docker service", map[string]interface{}{
		"id":   data.ID.ValueString(),
		"name": data.Name.ValueString(),
	})

	err := r.client.ServiceRemove(ctx, data.ID.ValueString())
	if err != nil {
		if strings.Contains(err.Error(), "No such service") || strings.Contains(err.Error(), "service not found") {
			tflog.Debug(ctx, "Service already removed", map[string]interface{}{
				"id": data.ID.ValueString(),
			})
			return
		}
		resp.Diagnostics.AddError(
			"Docker Service Deletion Failed",
			fmt.Sprintf("Failed to delete service %s: %s", data.ID.ValueString(), err),
		)
		return
	}

	tflog.Debug(ctx, "Deleted Docker service", map[string]interface{}{
		"id": data.ID.ValueString(),
	})
}

func (r *ServiceResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *ServiceResource) buildServiceSpec(ctx context.Context, data *ServiceResourceModel, diagnostics *diag.Diagnostics) (*swarm.ServiceSpec, error) {
	spec := &swarm.ServiceSpec{
		Annotations: swarm.Annotations{
			Name: data.Name.ValueString(),
		},
	}

	// Labels
	if !data.Labels.IsNull() {
		labels := make(map[string]string)
		diagnostics.Append(data.Labels.ElementsAs(ctx, &labels, false)...)
		spec.Labels = labels
	}

	// Mode
	if data.Mode.ValueString() == "global" {
		spec.Mode = swarm.ServiceMode{
			Global: &swarm.GlobalService{},
		}
	} else {
		replicas := uint64(data.Replicas.ValueInt64())
		spec.Mode = swarm.ServiceMode{
			Replicated: &swarm.ReplicatedService{
				Replicas: &replicas,
			},
		}
	}

	// Task Spec
	if !data.TaskSpec.IsNull() && len(data.TaskSpec.Elements()) > 0 {
		var taskSpecs []struct {
			ContainerSpec tftypes.List  `tfsdk:"container_spec"`
			Resources     tftypes.List  `tfsdk:"resources"`
			RestartPolicy tftypes.List  `tfsdk:"restart_policy"`
			Placement     tftypes.List  `tfsdk:"placement"`
			Networks      tftypes.Set   `tfsdk:"networks"`
			LogDriver     tftypes.List  `tfsdk:"log_driver"`
			ForceUpdate   tftypes.Int64 `tfsdk:"force_update"`
		}
		diagnostics.Append(data.TaskSpec.ElementsAs(ctx, &taskSpecs, false)...)
		if diagnostics.HasError() {
			return nil, fmt.Errorf("failed to parse task spec")
		}

		if len(taskSpecs) > 0 {
			taskSpec := taskSpecs[0]

			// Force update counter
			spec.TaskTemplate.ForceUpdate = uint64(taskSpec.ForceUpdate.ValueInt64())

			// Container spec
			if !taskSpec.ContainerSpec.IsNull() && len(taskSpec.ContainerSpec.Elements()) > 0 {
				var containerSpecs []struct {
					Image           tftypes.String `tfsdk:"image"`
					Command         tftypes.List   `tfsdk:"command"`
					Args            tftypes.List   `tfsdk:"args"`
					Hostname        tftypes.String `tfsdk:"hostname"`
					Env             tftypes.Map    `tfsdk:"env"`
					Dir             tftypes.String `tfsdk:"dir"`
					User            tftypes.String `tfsdk:"user"`
					Groups          tftypes.List   `tfsdk:"groups"`
					Privileges      tftypes.List   `tfsdk:"privileges"`
					ReadOnly        tftypes.Bool   `tfsdk:"read_only"`
					Mounts          tftypes.List   `tfsdk:"mounts"`
					StopSignal      tftypes.String `tfsdk:"stop_signal"`
					StopGracePeriod tftypes.String `tfsdk:"stop_grace_period"`
					Healthcheck     tftypes.List   `tfsdk:"healthcheck"`
					Hosts           tftypes.List   `tfsdk:"hosts"`
					DNSConfig       tftypes.List   `tfsdk:"dns_config"`
					Secrets         tftypes.List   `tfsdk:"secrets"`
					Configs         tftypes.List   `tfsdk:"configs"`
					Labels          tftypes.Map    `tfsdk:"labels"`
				}
				diagnostics.Append(taskSpec.ContainerSpec.ElementsAs(ctx, &containerSpecs, false)...)
				if diagnostics.HasError() {
					return nil, fmt.Errorf("failed to parse container spec")
				}

				if len(containerSpecs) > 0 {
					cs := containerSpecs[0]
					containerSpec := &swarm.ContainerSpec{
						Image:    cs.Image.ValueString(),
						ReadOnly: cs.ReadOnly.ValueBool(),
					}

					if !cs.Hostname.IsNull() {
						containerSpec.Hostname = cs.Hostname.ValueString()
					}

					if !cs.Dir.IsNull() {
						containerSpec.Dir = cs.Dir.ValueString()
					}

					if !cs.User.IsNull() {
						containerSpec.User = cs.User.ValueString()
					}

					if !cs.StopSignal.IsNull() {
						containerSpec.StopSignal = cs.StopSignal.ValueString()
					}

					// Environment variables
					if !cs.Env.IsNull() {
						envMap := make(map[string]string)
						diagnostics.Append(cs.Env.ElementsAs(ctx, &envMap, false)...)
						for k, v := range envMap {
							containerSpec.Env = append(containerSpec.Env, fmt.Sprintf("%s=%s", k, v))
						}
					}

					// Labels
					if !cs.Labels.IsNull() {
						labels := make(map[string]string)
						diagnostics.Append(cs.Labels.ElementsAs(ctx, &labels, false)...)
						containerSpec.Labels = labels
					}

					// Command
					if !cs.Command.IsNull() && len(cs.Command.Elements()) > 0 {
						var cmdItems []struct {
							Value tftypes.String `tfsdk:"value"`
						}
						diagnostics.Append(cs.Command.ElementsAs(ctx, &cmdItems, false)...)
						for _, item := range cmdItems {
							containerSpec.Command = append(containerSpec.Command, item.Value.ValueString())
						}
					}

					// Args
					if !cs.Args.IsNull() && len(cs.Args.Elements()) > 0 {
						var argItems []struct {
							Value tftypes.String `tfsdk:"value"`
						}
						diagnostics.Append(cs.Args.ElementsAs(ctx, &argItems, false)...)
						for _, item := range argItems {
							containerSpec.Args = append(containerSpec.Args, item.Value.ValueString())
						}
					}

					// Mounts
					if !cs.Mounts.IsNull() && len(cs.Mounts.Elements()) > 0 {
						var mountConfigs []struct {
							Target   tftypes.String `tfsdk:"target"`
							Source   tftypes.String `tfsdk:"source"`
							Type     tftypes.String `tfsdk:"type"`
							ReadOnly tftypes.Bool   `tfsdk:"read_only"`
						}
						diagnostics.Append(cs.Mounts.ElementsAs(ctx, &mountConfigs, false)...)
						for _, mc := range mountConfigs {
							containerSpec.Mounts = append(containerSpec.Mounts, mount.Mount{
								Target:   mc.Target.ValueString(),
								Source:   mc.Source.ValueString(),
								Type:     mount.Type(mc.Type.ValueString()),
								ReadOnly: mc.ReadOnly.ValueBool(),
							})
						}
					}

					// Secrets
					if !cs.Secrets.IsNull() && len(cs.Secrets.Elements()) > 0 {
						var secretRefs []struct {
							SecretID   tftypes.String `tfsdk:"secret_id"`
							SecretName tftypes.String `tfsdk:"secret_name"`
							FileName   tftypes.String `tfsdk:"file_name"`
							FileUID    tftypes.String `tfsdk:"file_uid"`
							FileGID    tftypes.String `tfsdk:"file_gid"`
							FileMode   tftypes.Int64  `tfsdk:"file_mode"`
						}
						diagnostics.Append(cs.Secrets.ElementsAs(ctx, &secretRefs, false)...)
						for _, sr := range secretRefs {
							mode := os.FileMode(sr.FileMode.ValueInt64())
							containerSpec.Secrets = append(containerSpec.Secrets, &swarm.SecretReference{
								SecretID:   sr.SecretID.ValueString(),
								SecretName: sr.SecretName.ValueString(),
								File: &swarm.SecretReferenceFileTarget{
									Name: sr.FileName.ValueString(),
									UID:  sr.FileUID.ValueString(),
									GID:  sr.FileGID.ValueString(),
									Mode: mode,
								},
							})
						}
					}

					// Configs
					if !cs.Configs.IsNull() && len(cs.Configs.Elements()) > 0 {
						var configRefs []struct {
							ConfigID   tftypes.String `tfsdk:"config_id"`
							ConfigName tftypes.String `tfsdk:"config_name"`
							FileName   tftypes.String `tfsdk:"file_name"`
							FileUID    tftypes.String `tfsdk:"file_uid"`
							FileGID    tftypes.String `tfsdk:"file_gid"`
							FileMode   tftypes.Int64  `tfsdk:"file_mode"`
						}
						diagnostics.Append(cs.Configs.ElementsAs(ctx, &configRefs, false)...)
						for _, cr := range configRefs {
							mode := os.FileMode(cr.FileMode.ValueInt64())
							containerSpec.Configs = append(containerSpec.Configs, &swarm.ConfigReference{
								ConfigID:   cr.ConfigID.ValueString(),
								ConfigName: cr.ConfigName.ValueString(),
								File: &swarm.ConfigReferenceFileTarget{
									Name: cr.FileName.ValueString(),
									UID:  cr.FileUID.ValueString(),
									GID:  cr.FileGID.ValueString(),
									Mode: mode,
								},
							})
						}
					}

					// Healthcheck
					if !cs.Healthcheck.IsNull() && len(cs.Healthcheck.Elements()) > 0 {
						var healthchecks []struct {
							Test        tftypes.List   `tfsdk:"test"`
							Interval    tftypes.String `tfsdk:"interval"`
							Timeout     tftypes.String `tfsdk:"timeout"`
							StartPeriod tftypes.String `tfsdk:"start_period"`
							Retries     tftypes.Int64  `tfsdk:"retries"`
						}
						diagnostics.Append(cs.Healthcheck.ElementsAs(ctx, &healthchecks, false)...)
						if len(healthchecks) > 0 {
							hc := healthchecks[0]
							var testCmd []string
							diagnostics.Append(hc.Test.ElementsAs(ctx, &testCmd, false)...)

							interval, _ := time.ParseDuration(hc.Interval.ValueString())
							timeout, _ := time.ParseDuration(hc.Timeout.ValueString())
							startPeriod, _ := time.ParseDuration(hc.StartPeriod.ValueString())

							containerSpec.Healthcheck = &container.HealthConfig{
								Test:        testCmd,
								Interval:    interval,
								Timeout:     timeout,
								StartPeriod: startPeriod,
								Retries:     int(hc.Retries.ValueInt64()),
							}
						}
					}

					spec.TaskTemplate.ContainerSpec = containerSpec
				}
			}

			// Networks
			if !taskSpec.Networks.IsNull() {
				var networks []string
				diagnostics.Append(taskSpec.Networks.ElementsAs(ctx, &networks, false)...)
				for _, network := range networks {
					spec.TaskTemplate.Networks = append(spec.TaskTemplate.Networks, swarm.NetworkAttachmentConfig{
						Target: network,
					})
				}
			}

			// Restart policy
			if !taskSpec.RestartPolicy.IsNull() && len(taskSpec.RestartPolicy.Elements()) > 0 {
				var restartPolicies []struct {
					Condition   tftypes.String `tfsdk:"condition"`
					Delay       tftypes.String `tfsdk:"delay"`
					MaxAttempts tftypes.Int64  `tfsdk:"max_attempts"`
					Window      tftypes.String `tfsdk:"window"`
				}
				diagnostics.Append(taskSpec.RestartPolicy.ElementsAs(ctx, &restartPolicies, false)...)
				if len(restartPolicies) > 0 {
					rp := restartPolicies[0]
					delay, _ := time.ParseDuration(rp.Delay.ValueString())
					spec.TaskTemplate.RestartPolicy = &swarm.RestartPolicy{
						Condition: swarm.RestartPolicyCondition(rp.Condition.ValueString()),
						Delay:     &delay,
					}
					if !rp.MaxAttempts.IsNull() {
						maxAttempts := uint64(rp.MaxAttempts.ValueInt64())
						spec.TaskTemplate.RestartPolicy.MaxAttempts = &maxAttempts
					}
					if !rp.Window.IsNull() {
						window, _ := time.ParseDuration(rp.Window.ValueString())
						spec.TaskTemplate.RestartPolicy.Window = &window
					}
				}
			}

			// Placement
			if !taskSpec.Placement.IsNull() && len(taskSpec.Placement.Elements()) > 0 {
				var placements []struct {
					Constraints tftypes.Set   `tfsdk:"constraints"`
					Prefs       tftypes.Set   `tfsdk:"prefs"`
					MaxReplicas tftypes.Int64 `tfsdk:"max_replicas"`
				}
				diagnostics.Append(taskSpec.Placement.ElementsAs(ctx, &placements, false)...)
				if len(placements) > 0 {
					p := placements[0]
					spec.TaskTemplate.Placement = &swarm.Placement{}
					if !p.Constraints.IsNull() {
						var constraints []string
						diagnostics.Append(p.Constraints.ElementsAs(ctx, &constraints, false)...)
						spec.TaskTemplate.Placement.Constraints = constraints
					}
					if !p.MaxReplicas.IsNull() {
						spec.TaskTemplate.Placement.MaxReplicas = uint64(p.MaxReplicas.ValueInt64())
					}
				}
			}

			// Log driver
			if !taskSpec.LogDriver.IsNull() && len(taskSpec.LogDriver.Elements()) > 0 {
				var logDrivers []struct {
					Name    tftypes.String `tfsdk:"name"`
					Options tftypes.Map    `tfsdk:"options"`
				}
				diagnostics.Append(taskSpec.LogDriver.ElementsAs(ctx, &logDrivers, false)...)
				if len(logDrivers) > 0 {
					ld := logDrivers[0]
					spec.TaskTemplate.LogDriver = &swarm.Driver{
						Name: ld.Name.ValueString(),
					}
					if !ld.Options.IsNull() {
						options := make(map[string]string)
						diagnostics.Append(ld.Options.ElementsAs(ctx, &options, false)...)
						spec.TaskTemplate.LogDriver.Options = options
					}
				}
			}

			// Resources
			if !taskSpec.Resources.IsNull() && len(taskSpec.Resources.Elements()) > 0 {
				var resources []struct {
					Limits       tftypes.List `tfsdk:"limits"`
					Reservations tftypes.List `tfsdk:"reservations"`
				}
				diagnostics.Append(taskSpec.Resources.ElementsAs(ctx, &resources, false)...)
				if len(resources) > 0 {
					res := resources[0]
					spec.TaskTemplate.Resources = &swarm.ResourceRequirements{}

					if !res.Limits.IsNull() && len(res.Limits.Elements()) > 0 {
						var limits []struct {
							NanoCPUs    tftypes.Int64 `tfsdk:"nano_cpus"`
							MemoryBytes tftypes.Int64 `tfsdk:"memory_bytes"`
						}
						diagnostics.Append(res.Limits.ElementsAs(ctx, &limits, false)...)
						if len(limits) > 0 {
							spec.TaskTemplate.Resources.Limits = &swarm.Limit{
								NanoCPUs:    limits[0].NanoCPUs.ValueInt64(),
								MemoryBytes: limits[0].MemoryBytes.ValueInt64(),
							}
						}
					}

					if !res.Reservations.IsNull() && len(res.Reservations.Elements()) > 0 {
						var reservations []struct {
							NanoCPUs    tftypes.Int64 `tfsdk:"nano_cpus"`
							MemoryBytes tftypes.Int64 `tfsdk:"memory_bytes"`
						}
						diagnostics.Append(res.Reservations.ElementsAs(ctx, &reservations, false)...)
						if len(reservations) > 0 {
							spec.TaskTemplate.Resources.Reservations = &swarm.Resources{
								NanoCPUs:    reservations[0].NanoCPUs.ValueInt64(),
								MemoryBytes: reservations[0].MemoryBytes.ValueInt64(),
							}
						}
					}
				}
			}
		}
	}

	// Endpoint spec
	if !data.EndpointSpec.IsNull() && len(data.EndpointSpec.Elements()) > 0 {
		var endpointSpecs []struct {
			Mode  tftypes.String `tfsdk:"mode"`
			Ports tftypes.List   `tfsdk:"ports"`
		}
		diagnostics.Append(data.EndpointSpec.ElementsAs(ctx, &endpointSpecs, false)...)
		if len(endpointSpecs) > 0 {
			es := endpointSpecs[0]
			spec.EndpointSpec = &swarm.EndpointSpec{
				Mode: swarm.ResolutionMode(es.Mode.ValueString()),
			}

			if !es.Ports.IsNull() && len(es.Ports.Elements()) > 0 {
				var ports []struct {
					Name          tftypes.String `tfsdk:"name"`
					Protocol      tftypes.String `tfsdk:"protocol"`
					TargetPort    tftypes.Int64  `tfsdk:"target_port"`
					PublishedPort tftypes.Int64  `tfsdk:"published_port"`
					PublishMode   tftypes.String `tfsdk:"publish_mode"`
				}
				diagnostics.Append(es.Ports.ElementsAs(ctx, &ports, false)...)
				for _, p := range ports {
					spec.EndpointSpec.Ports = append(spec.EndpointSpec.Ports, swarm.PortConfig{
						Name:          p.Name.ValueString(),
						Protocol:      swarm.PortConfigProtocol(p.Protocol.ValueString()),
						TargetPort:    uint32(p.TargetPort.ValueInt64()),
						PublishedPort: uint32(p.PublishedPort.ValueInt64()),
						PublishMode:   swarm.PortConfigPublishMode(p.PublishMode.ValueString()),
					})
				}
			}
		}
	}

	// Update config
	if !data.UpdateConfig.IsNull() && len(data.UpdateConfig.Elements()) > 0 {
		var updateConfigs []struct {
			Parallelism     tftypes.Int64   `tfsdk:"parallelism"`
			Delay           tftypes.String  `tfsdk:"delay"`
			FailureAction   tftypes.String  `tfsdk:"failure_action"`
			Monitor         tftypes.String  `tfsdk:"monitor"`
			MaxFailureRatio tftypes.Float64 `tfsdk:"max_failure_ratio"`
			Order           tftypes.String  `tfsdk:"order"`
		}
		diagnostics.Append(data.UpdateConfig.ElementsAs(ctx, &updateConfigs, false)...)
		if len(updateConfigs) > 0 {
			uc := updateConfigs[0]
			parallelism := uint64(uc.Parallelism.ValueInt64())
			delay, _ := time.ParseDuration(uc.Delay.ValueString())
			monitor, _ := time.ParseDuration(uc.Monitor.ValueString())
			spec.UpdateConfig = &swarm.UpdateConfig{
				Parallelism:     parallelism,
				Delay:           delay,
				FailureAction:   uc.FailureAction.ValueString(),
				Monitor:         monitor,
				MaxFailureRatio: float32(uc.MaxFailureRatio.ValueFloat64()),
				Order:           uc.Order.ValueString(),
			}
		}
	}

	// Rollback config
	if !data.RollbackConfig.IsNull() && len(data.RollbackConfig.Elements()) > 0 {
		var rollbackConfigs []struct {
			Parallelism     tftypes.Int64   `tfsdk:"parallelism"`
			Delay           tftypes.String  `tfsdk:"delay"`
			FailureAction   tftypes.String  `tfsdk:"failure_action"`
			Monitor         tftypes.String  `tfsdk:"monitor"`
			MaxFailureRatio tftypes.Float64 `tfsdk:"max_failure_ratio"`
			Order           tftypes.String  `tfsdk:"order"`
		}
		diagnostics.Append(data.RollbackConfig.ElementsAs(ctx, &rollbackConfigs, false)...)
		if len(rollbackConfigs) > 0 {
			rc := rollbackConfigs[0]
			parallelism := uint64(rc.Parallelism.ValueInt64())
			delay, _ := time.ParseDuration(rc.Delay.ValueString())
			monitor, _ := time.ParseDuration(rc.Monitor.ValueString())
			spec.RollbackConfig = &swarm.UpdateConfig{
				Parallelism:     parallelism,
				Delay:           delay,
				FailureAction:   rc.FailureAction.ValueString(),
				Monitor:         monitor,
				MaxFailureRatio: float32(rc.MaxFailureRatio.ValueFloat64()),
				Order:           rc.Order.ValueString(),
			}
		}
	}

	return spec, nil
}

func (r *ServiceResource) waitForConvergence(ctx context.Context, serviceID string, convergeConfig tftypes.List, diagnostics *diag.Diagnostics) {
	var configs []struct {
		Delay   tftypes.String `tfsdk:"delay"`
		Timeout tftypes.String `tfsdk:"timeout"`
	}
	diagnostics.Append(convergeConfig.ElementsAs(ctx, &configs, false)...)
	if diagnostics.HasError() || len(configs) == 0 {
		return
	}

	delay, _ := time.ParseDuration(configs[0].Delay.ValueString())
	timeout, _ := time.ParseDuration(configs[0].Timeout.ValueString())

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		service, _, err := r.client.ServiceInspectWithRaw(ctx, serviceID, types.ServiceInspectOptions{})
		if err != nil {
			tflog.Warn(ctx, "Failed to inspect service during convergence", map[string]interface{}{
				"error": err.Error(),
			})
			time.Sleep(delay)
			continue
		}

		// Check if service has converged
		if service.UpdateStatus == nil || service.UpdateStatus.State == swarm.UpdateStateCompleted {
			tflog.Debug(ctx, "Service converged", map[string]interface{}{
				"service_id": serviceID,
			})
			return
		}

		if service.UpdateStatus.State == swarm.UpdateStatePaused {
			diagnostics.AddWarning(
				"Service Update Paused",
				fmt.Sprintf("Service %s update is paused: %s", serviceID, service.UpdateStatus.Message),
			)
			return
		}

		time.Sleep(delay)
	}

	diagnostics.AddWarning(
		"Service Convergence Timeout",
		fmt.Sprintf("Service %s did not converge within the timeout period", serviceID),
	)
}
