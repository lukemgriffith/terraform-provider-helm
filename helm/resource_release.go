package helm

import (
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/helper/schema"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v1"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/downloader"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/strvals"
	// "k8s.io/helm/pkg/helm"
	// "k8s.io/helm/pkg/strvals"
	// "k8s.io/helm/pkg/chartutil"
	// "k8s.io/helm/pkg/downloader"
	// "k8s.io/helm/pkg/getter"
	// "k8s.io/helm/pkg/helm"
	// "k8s.io/helm/pkg/proto/hapi/chart"
	// "k8s.io/helm/pkg/proto/hapi/release"
	// "k8s.io/helm/pkg/repo"
	// "k8s.io/helm/pkg/strvals"
)

// ErrReleaseNotFound is the error when a Helm release is not found
var ErrReleaseNotFound = errors.New("release not found")

func resourceRelease() *schema.Resource {
	return &schema.Resource{
		Create: resourceReleaseCreate,
		Read:   resourceReleaseRead,
		//Delete:        resourceReleaseDelete,
		Update: resourceReleaseUpdate,
		Exists: resourceReleaseExists,
		//CustomizeDiff: resourceDiff,
		Schema: map[string]*schema.Schema{
			"name": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "Release name.",
			},
			"repository": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Repository where to locate the requested chart. If is a URL the chart is installed without installing the repository.",
			},
			"repository_key_file": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "The repositories cert key file",
			},
			"repository_cert_file": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "The repositories cert file",
			},
			"repository_ca_file": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "The Repositories CA File",
			},
			"chart": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "Chart name to be installed.",
			},
			"version": {
				Type:        schema.TypeString,
				Optional:    true,
				Computed:    true,
				Description: "Specify the exact chart version to install. If this is not specified, the latest version is installed.",
			},
			"devel": {
				Type:        schema.TypeBool,
				Optional:    true,
				Description: "Use chart development versions, too. Equivalent to version '>0.0.0-0'. If `version` is set, this is ignored",
				// Suppress changes of this attribute if `version` is set
				DiffSuppressFunc: func(k, old, new string, d *schema.ResourceData) bool {
					return d.Get("version").(string) != ""
				},
			},
			"values": {
				Type:        schema.TypeList,
				Optional:    true,
				Description: "List of values in raw yaml format to pass to helm.",
				Elem:        &schema.Schema{Type: schema.TypeString},
			},
			"set": {
				Type:        schema.TypeSet,
				Optional:    true,
				Description: "Custom values to be merged with the values.",
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"name": {
							Type:     schema.TypeString,
							Required: true,
						},
						"value": {
							Type:     schema.TypeString,
							Required: true,
						},
					},
				},
			},
			"set_sensitive": {
				Type:        schema.TypeSet,
				Optional:    true,
				Description: "Custom sensitive values to be merged with the values.",
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"name": {
							Type:     schema.TypeString,
							Required: true,
						},
						"value": {
							Type:      schema.TypeString,
							Required:  true,
							Sensitive: true,
						},
					},
				},
			},
			"set_string": {
				Type:        schema.TypeSet,
				Optional:    true,
				Description: "Custom string values to be merged with the values.",
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"name": {
							Type:     schema.TypeString,
							Required: true,
						},
						"value": {
							Type:     schema.TypeString,
							Required: true,
						},
					},
				},
			},
			"namespace": {
				Type:        schema.TypeString,
				Optional:    true,
				ForceNew:    true,
				Default:     "default",
				Description: "Namespace to install the release into.",
			},
			"verify": {
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     false,
				Description: "Verify the package before installing it.",
			},
			"keyring": {
				Type:        schema.TypeString,
				Optional:    true,
				Default:     os.ExpandEnv("$HOME/.gnupg/pubring.gpg"),
				Description: "Location of public keys used for verification. Used only if `verify` is true",
				// Suppress changes of this attribute if `verify` is false
				DiffSuppressFunc: func(k, old, new string, d *schema.ResourceData) bool {
					return !d.Get("verify").(bool)
				},
			},
			"timeout": {
				Type:        schema.TypeInt,
				Optional:    true,
				Default:     300,
				Description: "Time in seconds to wait for any individual kubernetes operation.",
			},
			"disable_webhooks": {
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     false,
				Description: "Prevent hooks from running.",
			},
			"reuse_values": {
				Type:        schema.TypeBool,
				Optional:    true,
				Description: "When upgrading, reuse the last release's values and merge in any overrides. If 'reset_values' is specified, this is ignored",
				Default:     false,
			},
			"reset_values": {
				Type:        schema.TypeBool,
				Optional:    true,
				Description: "When upgrading, reset the values to the ones built into the chart",
				Default:     false,
			},
			"force_update": {
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     false,
				Description: "Force resource update through delete/recreate if needed.",
			},
			"recreate_pods": {
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     false,
				Description: "Perform pods restart during upgrade/rollback",
			},
			"cleanup_on_fail": {
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     false,
				Description: "Allow deletion of new resources created in this upgrade when upgrade fails",
			},
			"max_history": {
				Type:        schema.TypeInt,
				Optional:    true,
				Default:     0,
				Description: "Limit the maximum number of revisions saved per release. Use 0 for no limit",
			},
			"atomic": {
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     false,
				Description: "If set, installation process purges chart on fail. The wait flag will be set automatically if atomic is used",
			},
			"skip_crds": {
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     false,
				Description: "If set, no CRDs will be installed. By default, CRDs are installed if not already present",
			},
			"render_subchart_notes": {
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     true,
				Description: "If set, render subchart notes along with the parent",
			},
			"wait": {
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     true,
				Description: "Will wait until all resources are in a ready state before marking the release as successful.",
			},
			"status": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "Status of the release.",
			},
			"dependency_update": {
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     false,
				Description: "run helm dependency update before installing the chart",
			},
			"metadata": {
				Type:        schema.TypeList,
				Computed:    true,
				Description: "Status of the deployed release.",
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"name": {
							Type:        schema.TypeString,
							Computed:    true,
							Description: "Name is the name of the release.",
						},
						"revision": {
							Type:        schema.TypeInt,
							Computed:    true,
							Description: "Version is an int32 which represents the version of the release.",
						},
						"namespace": {
							Type:        schema.TypeString,
							Computed:    true,
							Description: "Namespace is the kubernetes namespace of the release.",
						},
						"chart": {
							Type:        schema.TypeString,
							Computed:    true,
							Description: "The name of the chart.",
						},
						"version": {
							Type:        schema.TypeString,
							Computed:    true,
							Description: "A SemVer 2 conformant version string of the chart.",
						},
						"values": {
							Type:        schema.TypeString,
							Computed:    true,
							Description: "The raw yaml values used for the chart.",
						},
					},
				},
			},
		},
	}
}

func resourceReleaseRead(d *schema.ResourceData, meta interface{}) error {
	m := meta.(*Meta)
	c, err := m.GetHelmConfiguration()
	if err != nil {
		return err
	}

	name := d.Get("name").(string)

	r, err := getRelease(c, name)
	if err != nil {
		return err
	}

	return setIDAndMetadataFromRelease(d, r)
}

func resourceReleaseCreate(d *schema.ResourceData, meta interface{}) error {
	m := meta.(*Meta)
	actionConfig, err := m.GetHelmConfiguration()
	if err != nil {
		return err
	}

	name := d.Get("name").(string)

	repository := d.Get("repository").(string)
	repositoryURL, name, err := resolveChartName(repository, strings.TrimSpace(name))

	if err != nil {
		return err
	}
	version := getVersion(d, m)

	cpo := action.ChartPathOptions{
		CaFile:   d.Get("repository_ca_file").(string),
		CertFile: d.Get("repository_cert_file").(string),
		KeyFile:  d.Get("repository_key_file").(string),
		Keyring:  d.Get("keyring").(string),
		RepoURL:  repositoryURL,
		Verify:   d.Get("verify").(bool),
		Version:  version,
		//Username: string,
		//Password: string,
	}

	chart, path, err := getChart(d, m, name, cpo)
	if err != nil {
		return err
	}

	p := getter.All(m.Settings)

	values, err := getValues(d)
	if err != nil {
		return err
	}

	//
	validInstallableChart, err := isChartInstallable(chart)
	if !validInstallableChart {
		return err
	}

	updateDependency := d.Get("dependency_update").(bool)

	if req := chart.Metadata.Dependencies; req != nil {
		// If CheckDependencies returns an error, we have unfulfilled dependencies.
		// As of Helm 2.4.0, this is treated as a stopping condition:
		// https://github.com/helm/helm/issues/2209
		if err := action.CheckDependencies(chart, req); err != nil {
			if updateDependency {
				man := &downloader.Manager{
					Out:              os.Stdout,
					ChartPath:        path,
					Keyring:          d.Get("keyring").(string),
					SkipUpdate:       false,
					Getters:          p,
					RepositoryConfig: m.Settings.RepositoryConfig,
					RepositoryCache:  m.Settings.RepositoryCache,
				}
				if err := man.Update(); err != nil {
					return err
				}
			} else {
				return err
			}
		}
	}

	client := action.NewInstall(actionConfig)
	client.ChartPathOptions = cpo
	client.ClientOnly = false
	client.DryRun = false
	client.DisableHooks = d.Get("disable_webhooks").(bool)
	client.Replace = true
	client.Wait = d.Get("wait").(bool)
	client.Devel = d.Get("devel").(bool)
	client.DependencyUpdate = updateDependency
	client.Timeout = time.Duration(d.Get("timeout").(int32)) * time.Second
	client.Namespace = d.Get("namespace").(string)
	client.ReleaseName = d.Get("name").(string)
	client.GenerateName = false
	client.NameTemplate = ""
	client.OutputDir = ""
	client.Atomic = d.Get("atomic").(bool)
	client.SkipCRDs = d.Get("skip_crds").(bool)
	client.SubNotes = d.Get("render_subchart_notes").(bool)

	release, err := client.Run(chart, values)

	if err != nil {
		return err
	}

	return setIDAndMetadataFromRelease(d, release)
}

func resourceReleaseUpdate(d *schema.ResourceData, meta interface{}) error {
	m := meta.(*Meta)
	actionConfig, err := m.GetHelmConfiguration()
	if err != nil {
		return err
	}

	name := d.Get("name").(string)

	repository := d.Get("repository").(string)
	repositoryURL, name, err := resolveChartName(repository, strings.TrimSpace(name))

	if err != nil {
		return err
	}

	version := getVersion(d, m)
	cpo := action.ChartPathOptions{
		CaFile:   d.Get("repository_ca_file").(string),
		CertFile: d.Get("repository_cert_file").(string),
		KeyFile:  d.Get("repository_key_file").(string),
		Keyring:  d.Get("keyring").(string),
		RepoURL:  repositoryURL,
		Verify:   d.Get("verify").(bool),
		Version:  version,
		//Username: string,
		//Password: string,
	}

	chart, _, err := getChart(d, m, name, cpo)
	if err != nil {
		return err
	}

	if req := chart.Metadata.Dependencies; req != nil {
		if err := action.CheckDependencies(chart, req); err != nil {
			return err
		}
	}

	client := action.NewUpgrade(actionConfig)
	client.ChartPathOptions = cpo
	client.Devel = d.Get("devel").(bool)
	client.Namespace = d.Get("namespace").(string)
	client.Timeout = time.Duration(d.Get("timeout").(int32)) * time.Second
	client.Wait = d.Get("wait").(bool)
	client.DryRun = false
	client.DisableHooks = d.Get("disable_webhooks").(bool)
	client.Atomic = d.Get("atomic").(bool)
	client.SubNotes = d.Get("render_subchart_notes").(bool)
	client.Force = d.Get("force_update").(bool)
	client.ResetValues = d.Get("reset_values").(bool)
	client.ReuseValues = d.Get("reuse_values").(bool)
	client.Recreate = d.Get("recreate_pods").(bool)
	client.MaxHistory = d.Get("max_history").(int)
	client.CleanupOnFail = d.Get("cleanup_on_fail").(bool)

	values, err := getValues(d)
	if err != nil {
		return err
	}

	release, err := client.Run(name, chart, values)
	if err != nil {
		return err
	}

	return setIDAndMetadataFromRelease(d, release)
}

// func resourceReleaseDelete(d *schema.ResourceData, meta interface{}) error {
// 	m := meta.(*Meta)
// 	c, err := m.GetHelmClient()
// 	if err != nil {
// 		return err
// 	}

// 	name := d.Get("name").(string)
// 	disableWebhooks := d.Get("disable_webhooks").(bool)
// 	timeout := int64(d.Get("timeout").(int))

// 	if err := deleteRelease(c, name, disableWebhooks, timeout); err != nil {
// 		return err
// 	}
// 	d.SetId("")
// 	return nil
// }

func setIDAndMetadataFromRelease(d *schema.ResourceData, r *release.Release) error {
	d.SetId(r.Name)
	d.Set("version", r.Chart.Metadata.Version)
	d.Set("namespace", r.Namespace)
	d.Set("status", r.Info.Status.String())

	c, err := json.Marshal(r.Config)

	if err != nil {
		return err
	}

	return d.Set("metadata", []map[string]interface{}{{
		"name":      r.Name,
		"revision":  r.Version,
		"namespace": r.Namespace,
		"chart":     r.Chart.Metadata.Name,
		"version":   r.Chart.Metadata.Version,
		"values":    c,
	}})
}

func resourceReleaseExists(d *schema.ResourceData, meta interface{}) (bool, error) {
	m := meta.(*Meta)

	c, err := m.GetHelmConfiguration()
	if err != nil {
		return false, err
	}

	name := d.Get("name").(string)
	_, err = getRelease(c, name)

	if err == nil {
		return true, nil
	}

	if err == ErrReleaseNotFound {
		return false, nil
	}

	return false, err
}

// func deleteRelease(c helm.Interface, name string, disableWebhooks bool, timeout int64) error {

// 	opts := []helm.DeleteOption{
// 		helm.DeleteDisableHooks(disableWebhooks),
// 		helm.DeletePurge(true),
// 		helm.DeleteTimeout(timeout),
// 	}

// 	if _, err := c.DeleteRelease(name, opts...); err != nil {
// 		return err
// 	}

// 	return nil
// }

type resourceGetter interface {
	Get(string) interface{}
}

func getVersion(d resourceGetter, m *Meta) (version string) {
	version = d.Get("version").(string)

	if version == "" && d.Get("devel").(bool) {
		debug("setting version to >0.0.0-0")
		version = ">0.0.0-0"
	} else {
		version = strings.TrimSpace(version)
	}

	return
}

func getChart(d resourceGetter, m *Meta, name string, cpo action.ChartPathOptions) (c *chart.Chart, path string, err error) {

	n, err := cpo.LocateChart(name, m.Settings)

	if err != nil {
		return nil, "", err
	}

	c, err = loader.Load(n)

	return c, path, nil
}

// Merges source and destination map, preferring values from the source map
// Taken from github.com/helm/cmd/install.go
func mergeValues(dest map[string]interface{}, src map[string]interface{}) map[string]interface{} {
	for k, v := range src {
		// If the key doesn't exist already, then just set the key to that value
		if _, exists := dest[k]; !exists {
			dest[k] = v
			continue
		}
		nextMap, ok := v.(map[string]interface{})
		// If it isn't another map, overwrite the value
		if !ok {
			dest[k] = v
			continue
		}
		// If the key doesn't exist already, then just set the key to that value
		if _, exists := dest[k]; !exists {
			dest[k] = nextMap
			continue
		}
		// Edge case: If the key exists in the destination, but isn't a map
		destMap, isMap := dest[k].(map[string]interface{})
		// If the source map has a map for this key, prefer it
		if !isMap {
			dest[k] = v
			continue
		}
		// If we got to this point, it is a map in both, so merge them
		dest[k] = mergeValues(destMap, nextMap)
	}
	return dest
}

func getValues(d *schema.ResourceData) (map[string]interface{}, error) {
	base := map[string]interface{}{}

	for _, raw := range d.Get("values").([]interface{}) {
		if raw != nil {
			values := raw.(string)
			if values != "" {
				currentMap := map[string]interface{}{}
				if err := yaml.Unmarshal([]byte(values), &currentMap); err != nil {
					return nil, fmt.Errorf("---> %v %s", err, values)
				}
				base = mergeValues(base, currentMap)
			}
		}
	}

	for _, raw := range d.Get("set").(*schema.Set).List() {
		set := raw.(map[string]interface{})

		name := set["name"].(string)
		value := set["value"].(string)

		if err := strvals.ParseInto(fmt.Sprintf("%s=%s", name, value), base); err != nil {
			return nil, fmt.Errorf("failed parsing key %q with value %s, %s", name, value, err)
		}
	}

	for _, raw := range d.Get("set_sensitive").(*schema.Set).List() {
		set := raw.(map[string]interface{})

		name := set["name"].(string)
		value := set["value"].(string)

		if err := strvals.ParseInto(fmt.Sprintf("%s=%s", name, value), base); err != nil {
			return nil, fmt.Errorf("failed parsing key %q with sensitive value, %s", name, err)
		}
	}

	for _, raw := range d.Get("set_string").(*schema.Set).List() {
		set := raw.(map[string]interface{})

		name := set["name"].(string)
		value := set["value"].(string)

		if err := strvals.ParseIntoString(fmt.Sprintf("%s=%s", name, value), base); err != nil {
			return nil, fmt.Errorf("failed parsing key %q with value %s, %s", name, value, err)
		}
	}

	yaml, err := yaml.Marshal(base)
	if err == nil {
		yamlString := string(yaml)
		for _, raw := range d.Get("set_sensitive").(*schema.Set).List() {
			set := raw.(map[string]interface{})
			yamlString = strings.Replace(yamlString, set["value"].(string), "<SENSITIVE>", -1)
		}

		log.Printf("---[ values.yaml ]-----------------------------------\n%s\n", yamlString)
	}

	return base, err
}

func getRelease(cfg *action.Configuration, name string) (*release.Release, error) {

	get := action.NewGet(cfg)
	res, err := get.Run(name)

	if err != nil {
		if strings.Contains(err.Error(), "release: not found") {
			return nil, ErrReleaseNotFound
		}

		debug("could not get release %s", err)

		return nil, err
	}

	debug("got release %v", res)

	return res, nil
}

func resolveChartName(repository, name string) (string, string, error) {
	_, err := url.ParseRequestURI(repository)
	if err == nil {
		return repository, name, nil
	}

	if strings.Index(name, "/") == -1 && repository != "" {
		name = fmt.Sprintf("%s/%s", repository, name)
	}

	return "", name, nil
}

func isChartInstallable(ch *chart.Chart) (bool, error) {
	switch ch.Metadata.Type {
	case "", "application":
		return true, nil
	}
	return false, errors.Errorf("%s charts are not installable", ch.Metadata.Type)
}
