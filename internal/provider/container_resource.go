package provider

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/go-connections/nat"
	"github.com/elioseverojunior/terraform-provider-docker/internal/docker"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var (
	_ resource.Resource                = &ContainerResource{}
	_ resource.ResourceWithImportState = &ContainerResource{}
)

type ContainerResource struct {
	client *docker.Client
}

type ContainerResourceModel struct {
	ID          types.String      `tfsdk:"id"`
	Name        types.String      `tfsdk:"name"`
	Image       types.String      `tfsdk:"image"`
	Command     types.List        `tfsdk:"command"`
	Entrypoint  types.List        `tfsdk:"entrypoint"`
	Env         types.Map         `tfsdk:"env"`
	Labels      types.Map         `tfsdk:"labels"`
	Hostname    types.String      `tfsdk:"hostname"`
	Domainname  types.String      `tfsdk:"domainname"`
	User        types.String      `tfsdk:"user"`
	WorkingDir  types.String      `tfsdk:"working_dir"`
	Restart     types.String      `tfsdk:"restart"`
	Privileged  types.Bool        `tfsdk:"privileged"`
	Tty         types.Bool        `tfsdk:"tty"`
	StdinOpen   types.Bool        `tfsdk:"stdin_open"`
	NetworkMode types.String      `tfsdk:"network_mode"`
	DNS         types.List        `tfsdk:"dns"`
	DNSSearch   types.List        `tfsdk:"dns_search"`
	ExtraHosts  types.List        `tfsdk:"extra_hosts"`
	Memory      types.Int64       `tfsdk:"memory"`
	MemorySwap  types.Int64       `tfsdk:"memory_swap"`
	CPUShares   types.Int64       `tfsdk:"cpu_shares"`
	CPUPeriod   types.Int64       `tfsdk:"cpu_period"`
	CPUQuota    types.Int64       `tfsdk:"cpu_quota"`
	Remove      types.Bool        `tfsdk:"remove"`
	MustRun     types.Bool        `tfsdk:"must_run"`
	Ports       []PortModel       `tfsdk:"ports"`
	Volumes     []VolumeModel     `tfsdk:"volumes"`
	Networks    types.Set         `tfsdk:"networks"`
	Healthcheck *HealthcheckModel `tfsdk:"healthcheck"`
	ContainerID types.String      `tfsdk:"container_id"`
	IPAddress   types.String      `tfsdk:"ip_address"`
	Gateway     types.String      `tfsdk:"gateway"`
	ExitCode    types.Int64       `tfsdk:"exit_code"`
}

type PortModel struct {
	Internal types.Int64  `tfsdk:"internal"`
	External types.Int64  `tfsdk:"external"`
	IP       types.String `tfsdk:"ip"`
	Protocol types.String `tfsdk:"protocol"`
}

type VolumeModel struct {
	VolumeName    types.String `tfsdk:"volume_name"`
	HostPath      types.String `tfsdk:"host_path"`
	ContainerPath types.String `tfsdk:"container_path"`
	ReadOnly      types.Bool   `tfsdk:"read_only"`
}

type HealthcheckModel struct {
	Test        types.List   `tfsdk:"test"`
	Interval    types.String `tfsdk:"interval"`
	Timeout     types.String `tfsdk:"timeout"`
	StartPeriod types.String `tfsdk:"start_period"`
	Retries     types.Int64  `tfsdk:"retries"`
}

func NewContainerResource() resource.Resource {
	return &ContainerResource{}
}

func (r *ContainerResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_container"
}

func (r *ContainerResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages Docker containers.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The ID of this resource.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "The name of the container.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"image": schema.StringAttribute{
				Description: "The ID or name of the image to use for the container.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"command": schema.ListAttribute{
				Description: "The command to run in the container.",
				Optional:    true,
				ElementType: types.StringType,
			},
			"entrypoint": schema.ListAttribute{
				Description: "The entrypoint for the container.",
				Optional:    true,
				ElementType: types.StringType,
			},
			"env": schema.MapAttribute{
				Description: "Environment variables to set in the container.",
				Optional:    true,
				ElementType: types.StringType,
			},
			"labels": schema.MapAttribute{
				Description: "User-defined key/value metadata.",
				Optional:    true,
				ElementType: types.StringType,
			},
			"hostname": schema.StringAttribute{
				Description: "Hostname to set for the container.",
				Optional:    true,
			},
			"domainname": schema.StringAttribute{
				Description: "Domain name for the container.",
				Optional:    true,
			},
			"user": schema.StringAttribute{
				Description: "User that commands are run as inside the container.",
				Optional:    true,
			},
			"working_dir": schema.StringAttribute{
				Description: "Working directory inside the container.",
				Optional:    true,
			},
			"restart": schema.StringAttribute{
				Description: "Restart policy for the container. Values are: no, on-failure[:max-retries], always, unless-stopped.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString("no"),
			},
			"privileged": schema.BoolAttribute{
				Description: "Run container in privileged mode.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"tty": schema.BoolAttribute{
				Description: "Allocate a pseudo-TTY.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"stdin_open": schema.BoolAttribute{
				Description: "Keep STDIN open even if not attached.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"network_mode": schema.StringAttribute{
				Description: "Network mode of the container (bridge, host, none, container:<name|id>).",
				Optional:    true,
			},
			"dns": schema.ListAttribute{
				Description: "Set of DNS servers.",
				Optional:    true,
				ElementType: types.StringType,
			},
			"dns_search": schema.ListAttribute{
				Description: "Set of DNS search domains.",
				Optional:    true,
				ElementType: types.StringType,
			},
			"extra_hosts": schema.ListAttribute{
				Description: "A list of hostnames/IP mappings to add to the container's /etc/hosts file. Format: hostname:IP.",
				Optional:    true,
				ElementType: types.StringType,
			},
			"memory": schema.Int64Attribute{
				Description: "Memory limit in bytes.",
				Optional:    true,
				Computed:    true,
				Default:     int64default.StaticInt64(0),
			},
			"memory_swap": schema.Int64Attribute{
				Description: "Total memory limit (memory + swap) in bytes. Set to -1 for unlimited swap.",
				Optional:    true,
				Computed:    true,
				Default:     int64default.StaticInt64(0),
			},
			"cpu_shares": schema.Int64Attribute{
				Description: "CPU shares (relative weight).",
				Optional:    true,
				Computed:    true,
				Default:     int64default.StaticInt64(0),
			},
			"cpu_period": schema.Int64Attribute{
				Description: "CPU CFS period in microseconds.",
				Optional:    true,
				Computed:    true,
				Default:     int64default.StaticInt64(0),
			},
			"cpu_quota": schema.Int64Attribute{
				Description: "CPU CFS quota in microseconds.",
				Optional:    true,
				Computed:    true,
				Default:     int64default.StaticInt64(0),
			},
			"remove": schema.BoolAttribute{
				Description: "If true, removes the container on destruction. Default is true.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(true),
			},
			"must_run": schema.BoolAttribute{
				Description: "If true, ensures the container is running. Default is true.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(true),
			},
			"networks": schema.SetAttribute{
				Description: "Set of networks to attach to the container.",
				Optional:    true,
				ElementType: types.StringType,
			},
			"container_id": schema.StringAttribute{
				Description: "The Docker container ID.",
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
			"exit_code": schema.Int64Attribute{
				Description: "The exit code of the container if it has stopped.",
				Computed:    true,
			},
		},
		Blocks: map[string]schema.Block{
			"ports": schema.ListNestedBlock{
				Description: "Port mappings for the container.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"internal": schema.Int64Attribute{
							Description: "Internal port within the container.",
							Required:    true,
						},
						"external": schema.Int64Attribute{
							Description: "External port on the host.",
							Optional:    true,
						},
						"ip": schema.StringAttribute{
							Description: "IP address to bind the port on.",
							Optional:    true,
							Computed:    true,
							Default:     stringdefault.StaticString("0.0.0.0"),
						},
						"protocol": schema.StringAttribute{
							Description: "Protocol for the port (tcp/udp). Default is tcp.",
							Optional:    true,
							Computed:    true,
							Default:     stringdefault.StaticString("tcp"),
						},
					},
				},
			},
			"volumes": schema.ListNestedBlock{
				Description: "Volume mounts for the container.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"volume_name": schema.StringAttribute{
							Description: "The name of the Docker volume to mount.",
							Optional:    true,
						},
						"host_path": schema.StringAttribute{
							Description: "The path on the host to bind mount.",
							Optional:    true,
						},
						"container_path": schema.StringAttribute{
							Description: "The path inside the container to mount to.",
							Required:    true,
						},
						"read_only": schema.BoolAttribute{
							Description: "Mount the volume as read-only.",
							Optional:    true,
							Computed:    true,
							Default:     booldefault.StaticBool(false),
						},
					},
				},
			},
			"healthcheck": schema.SingleNestedBlock{
				Description: "Health check configuration.",
				Attributes: map[string]schema.Attribute{
					"test": schema.ListAttribute{
						Description: "Command to run to check health.",
						Required:    true,
						ElementType: types.StringType,
					},
					"interval": schema.StringAttribute{
						Description: "Time between running the check (e.g., 30s, 1m).",
						Optional:    true,
						Computed:    true,
						Default:     stringdefault.StaticString("30s"),
					},
					"timeout": schema.StringAttribute{
						Description: "Maximum time to wait for a check (e.g., 10s).",
						Optional:    true,
						Computed:    true,
						Default:     stringdefault.StaticString("30s"),
					},
					"start_period": schema.StringAttribute{
						Description: "Start period for the container to initialize before checks are retried (e.g., 5s).",
						Optional:    true,
						Computed:    true,
						Default:     stringdefault.StaticString("0s"),
					},
					"retries": schema.Int64Attribute{
						Description: "Number of consecutive failures needed to consider a container as unhealthy.",
						Optional:    true,
						Computed:    true,
						Default:     int64default.StaticInt64(3),
					},
				},
			},
		},
	}
}

func (r *ContainerResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *ContainerResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data ContainerResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	containerName := data.Name.ValueString()
	tflog.Debug(ctx, "Creating Docker container", map[string]interface{}{
		"name":  containerName,
		"image": data.Image.ValueString(),
	})

	// Build container config
	containerConfig := &container.Config{
		Image:     data.Image.ValueString(),
		Tty:       data.Tty.ValueBool(),
		OpenStdin: data.StdinOpen.ValueBool(),
	}

	// Hostname
	if !data.Hostname.IsNull() {
		containerConfig.Hostname = data.Hostname.ValueString()
	}

	// Domainname
	if !data.Domainname.IsNull() {
		containerConfig.Domainname = data.Domainname.ValueString()
	}

	// User
	if !data.User.IsNull() {
		containerConfig.User = data.User.ValueString()
	}

	// Working directory
	if !data.WorkingDir.IsNull() {
		containerConfig.WorkingDir = data.WorkingDir.ValueString()
	}

	// Command
	if !data.Command.IsNull() {
		var cmd []string
		resp.Diagnostics.Append(data.Command.ElementsAs(ctx, &cmd, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		containerConfig.Cmd = cmd
	}

	// Entrypoint
	if !data.Entrypoint.IsNull() {
		var entrypoint []string
		resp.Diagnostics.Append(data.Entrypoint.ElementsAs(ctx, &entrypoint, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		containerConfig.Entrypoint = entrypoint
	}

	// Environment variables
	if !data.Env.IsNull() {
		envMap := make(map[string]string)
		resp.Diagnostics.Append(data.Env.ElementsAs(ctx, &envMap, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		var envList []string
		for k, v := range envMap {
			envList = append(envList, fmt.Sprintf("%s=%s", k, v))
		}
		containerConfig.Env = envList
	}

	// Labels
	if !data.Labels.IsNull() {
		labels := make(map[string]string)
		resp.Diagnostics.Append(data.Labels.ElementsAs(ctx, &labels, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		containerConfig.Labels = labels
	}

	// Healthcheck
	if data.Healthcheck != nil {
		var test []string
		resp.Diagnostics.Append(data.Healthcheck.Test.ElementsAs(ctx, &test, false)...)
		if resp.Diagnostics.HasError() {
			return
		}

		healthConfig := &container.HealthConfig{
			Test:    test,
			Retries: int(data.Healthcheck.Retries.ValueInt64()),
		}

		if interval, err := parseDuration(data.Healthcheck.Interval.ValueString()); err == nil {
			healthConfig.Interval = time.Duration(interval)
		}
		if timeout, err := parseDuration(data.Healthcheck.Timeout.ValueString()); err == nil {
			healthConfig.Timeout = time.Duration(timeout)
		}
		if startPeriod, err := parseDuration(data.Healthcheck.StartPeriod.ValueString()); err == nil {
			healthConfig.StartPeriod = time.Duration(startPeriod)
		}

		containerConfig.Healthcheck = healthConfig
	}

	// Exposed ports
	exposedPorts := nat.PortSet{}
	portBindings := nat.PortMap{}

	for _, port := range data.Ports {
		internalPort := fmt.Sprintf("%d/%s", port.Internal.ValueInt64(), port.Protocol.ValueString())
		natPort := nat.Port(internalPort)
		exposedPorts[natPort] = struct{}{}

		binding := nat.PortBinding{
			HostIP: port.IP.ValueString(),
		}
		if !port.External.IsNull() && port.External.ValueInt64() > 0 {
			binding.HostPort = fmt.Sprintf("%d", port.External.ValueInt64())
		}
		portBindings[natPort] = append(portBindings[natPort], binding)
	}

	containerConfig.ExposedPorts = exposedPorts

	// Build host config
	hostConfig := &container.HostConfig{
		Privileged:   data.Privileged.ValueBool(),
		PortBindings: portBindings,
	}

	// Restart policy
	restart := data.Restart.ValueString()
	if restart != "" {
		policy := container.RestartPolicy{Name: container.RestartPolicyMode(restart)}
		if strings.HasPrefix(restart, "on-failure") {
			parts := strings.Split(restart, ":")
			if len(parts) == 2 {
				var maxRetries int
				fmt.Sscanf(parts[1], "%d", &maxRetries)
				policy.Name = container.RestartPolicyOnFailure
				policy.MaximumRetryCount = maxRetries
			}
		}
		hostConfig.RestartPolicy = policy
	}

	// Network mode
	if !data.NetworkMode.IsNull() {
		hostConfig.NetworkMode = container.NetworkMode(data.NetworkMode.ValueString())
	}

	// DNS
	if !data.DNS.IsNull() {
		var dns []string
		resp.Diagnostics.Append(data.DNS.ElementsAs(ctx, &dns, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		hostConfig.DNS = dns
	}

	// DNS search
	if !data.DNSSearch.IsNull() {
		var dnsSearch []string
		resp.Diagnostics.Append(data.DNSSearch.ElementsAs(ctx, &dnsSearch, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		hostConfig.DNSSearch = dnsSearch
	}

	// Extra hosts
	if !data.ExtraHosts.IsNull() {
		var extraHosts []string
		resp.Diagnostics.Append(data.ExtraHosts.ElementsAs(ctx, &extraHosts, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		hostConfig.ExtraHosts = extraHosts
	}

	// Resource limits
	if data.Memory.ValueInt64() > 0 {
		hostConfig.Memory = data.Memory.ValueInt64()
	}
	if data.MemorySwap.ValueInt64() != 0 {
		hostConfig.MemorySwap = data.MemorySwap.ValueInt64()
	}
	if data.CPUShares.ValueInt64() > 0 {
		hostConfig.CPUShares = data.CPUShares.ValueInt64()
	}
	if data.CPUPeriod.ValueInt64() > 0 {
		hostConfig.CPUPeriod = data.CPUPeriod.ValueInt64()
	}
	if data.CPUQuota.ValueInt64() > 0 {
		hostConfig.CPUQuota = data.CPUQuota.ValueInt64()
	}

	// Volume mounts
	var mounts []mount.Mount
	for _, vol := range data.Volumes {
		m := mount.Mount{
			Target:   vol.ContainerPath.ValueString(),
			ReadOnly: vol.ReadOnly.ValueBool(),
		}

		if !vol.VolumeName.IsNull() && vol.VolumeName.ValueString() != "" {
			m.Type = mount.TypeVolume
			m.Source = vol.VolumeName.ValueString()
		} else if !vol.HostPath.IsNull() && vol.HostPath.ValueString() != "" {
			m.Type = mount.TypeBind
			m.Source = vol.HostPath.ValueString()
		}

		mounts = append(mounts, m)
	}
	hostConfig.Mounts = mounts

	// Network config
	networkConfig := &network.NetworkingConfig{}
	if !data.Networks.IsNull() {
		var networks []string
		resp.Diagnostics.Append(data.Networks.ElementsAs(ctx, &networks, false)...)
		if resp.Diagnostics.HasError() {
			return
		}

		if len(networks) > 0 {
			networkConfig.EndpointsConfig = make(map[string]*network.EndpointSettings)
			for _, net := range networks {
				networkConfig.EndpointsConfig[net] = &network.EndpointSettings{}
			}
		}
	}

	// Create container
	containerResp, err := r.client.ContainerCreate(ctx, containerConfig, hostConfig, networkConfig, nil, containerName)
	if err != nil {
		resp.Diagnostics.AddError("Container Create Error", fmt.Sprintf("Unable to create container %s: %s", containerName, err))
		return
	}

	data.ID = types.StringValue(containerResp.ID)
	data.ContainerID = types.StringValue(containerResp.ID)

	// Start container if must_run is true
	if data.MustRun.ValueBool() {
		err = r.client.ContainerStart(ctx, containerResp.ID, container.StartOptions{})
		if err != nil {
			resp.Diagnostics.AddError("Container Start Error", fmt.Sprintf("Unable to start container %s: %s", containerName, err))
			return
		}
	}

	// Refresh state with computed values
	r.readContainerState(ctx, &data, resp)

	tflog.Debug(ctx, "Created Docker container", map[string]interface{}{
		"name": containerName,
		"id":   containerResp.ID,
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ContainerResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data ContainerResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	containerID := data.ID.ValueString()

	containerJSON, err := r.client.ContainerInspect(ctx, containerID)
	if err != nil {
		if strings.Contains(err.Error(), "No such container") || strings.Contains(err.Error(), "not found") {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Container Read Error", fmt.Sprintf("Unable to read container %s: %s", containerID, err))
		return
	}

	data.Name = types.StringValue(strings.TrimPrefix(containerJSON.Name, "/"))
	data.Image = types.StringValue(containerJSON.Config.Image)
	data.ContainerID = types.StringValue(containerJSON.ID)

	// Network info
	if containerJSON.NetworkSettings != nil {
		if containerJSON.NetworkSettings.IPAddress != "" {
			data.IPAddress = types.StringValue(containerJSON.NetworkSettings.IPAddress)
		}
		if containerJSON.NetworkSettings.Gateway != "" {
			data.Gateway = types.StringValue(containerJSON.NetworkSettings.Gateway)
		}
	}

	// Exit code
	if containerJSON.State != nil {
		data.ExitCode = types.Int64Value(int64(containerJSON.State.ExitCode))
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ContainerResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data ContainerResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Most container changes require recreation
	// Only save state for non-recreating updates

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ContainerResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data ContainerResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !data.Remove.ValueBool() {
		tflog.Debug(ctx, "Not removing container as configured", map[string]interface{}{
			"name": data.Name.ValueString(),
		})
		return
	}

	containerID := data.ID.ValueString()

	tflog.Debug(ctx, "Stopping Docker container", map[string]interface{}{
		"id": containerID,
	})

	// Stop container first
	timeout := 30
	err := r.client.ContainerStop(ctx, containerID, container.StopOptions{Timeout: &timeout})
	if err != nil && !strings.Contains(err.Error(), "is not running") && !strings.Contains(err.Error(), "No such container") {
		resp.Diagnostics.AddError("Container Stop Error", fmt.Sprintf("Unable to stop container %s: %s", containerID, err))
		return
	}

	tflog.Debug(ctx, "Removing Docker container", map[string]interface{}{
		"id": containerID,
	})

	// Remove container
	err = r.client.ContainerRemove(ctx, containerID, container.RemoveOptions{
		Force:         true,
		RemoveVolumes: false,
	})
	if err != nil {
		if strings.Contains(err.Error(), "No such container") || strings.Contains(err.Error(), "not found") {
			return
		}
		resp.Diagnostics.AddError("Container Delete Error", fmt.Sprintf("Unable to delete container %s: %s", containerID, err))
		return
	}

	tflog.Debug(ctx, "Deleted Docker container", map[string]interface{}{
		"id": containerID,
	})
}

func (r *ContainerResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *ContainerResource) readContainerState(ctx context.Context, data *ContainerResourceModel, resp *resource.CreateResponse) {
	containerJSON, err := r.client.ContainerInspect(ctx, data.ID.ValueString())
	if err != nil {
		return
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

	// Exit code
	if containerJSON.State != nil {
		data.ExitCode = types.Int64Value(int64(containerJSON.State.ExitCode))
	}
}

func parseDuration(s string) (int64, error) {
	// Simple duration parser for Docker health check intervals
	// Accepts formats like "30s", "1m", "5m30s"
	if s == "" {
		return 0, fmt.Errorf("empty duration")
	}

	var duration int64
	var num int64
	for _, c := range s {
		if c >= '0' && c <= '9' {
			num = num*10 + int64(c-'0')
		} else {
			switch c {
			case 's':
				duration += num * 1e9
			case 'm':
				duration += num * 60 * 1e9
			case 'h':
				duration += num * 3600 * 1e9
			}
			num = 0
		}
	}

	return duration, nil
}
