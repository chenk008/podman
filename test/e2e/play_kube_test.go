package integration

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"

	"github.com/containers/podman/v3/pkg/util"
	. "github.com/containers/podman/v3/test/utils"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/opencontainers/selinux/go-selinux"
)

var unknownKindYaml = `
apiVersion: v1
kind: UnknownKind
metadata:
  labels:
    app: app1
  name: unknown
spec:
  hostname: unknown
`

var selinuxLabelPodYaml = `
apiVersion: v1
kind: Pod
metadata:
  creationTimestamp: "2021-02-02T22:18:20Z"
  labels:
    app: label-pod
  name: label-pod
spec:
  containers:
  - command:
    - top
    - -d
    - "1.5"
    env:
    - name: PATH
      value: /usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
    - name: TERM
      value: xterm
    - name: container
      value: podman
    - name: HOSTNAME
      value: label-pod
    image: quay.io/libpod/alpine:latest
    name: test
    securityContext:
      allowPrivilegeEscalation: true
      capabilities:
        drop:
        - CAP_MKNOD
        - CAP_NET_RAW
        - CAP_AUDIT_WRITE
      privileged: false
      readOnlyRootFilesystem: false
      seLinuxOptions:
        user: unconfined_u
        role: system_r
        type: spc_t
        level: s0
    workingDir: /
status: {}
`

var configMapYamlTemplate = `
apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ .Name }}
data:
{{ with .Data }}
  {{ range $key, $value := . }}
    {{ $key }}: {{ $value }}
  {{ end }}
{{ end }}
`

var persistentVolumeClaimYamlTemplate = `
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: {{ .Name }}
{{ with .Annotations }}
  annotations:
  {{ range $key, $value := . }}
    {{ $key }}: {{ $value }}
  {{ end }}
{{ end }}
spec:
  accessModes:
    - "ReadWriteOnce"
  resources:
    requests:
      storage: "1Gi"
  storageClassName: default
`

var podYamlTemplate = `
apiVersion: v1
kind: Pod
metadata:
  creationTimestamp: "2019-07-17T14:44:08Z"
  name: {{ .Name }}
  labels:
    app: {{ .Name }}
{{ with .Labels }}
  {{ range $key, $value := . }}
    {{ $key }}: {{ $value }}
  {{ end }}
{{ end }}
{{ with .Annotations }}
  annotations:
  {{ range $key, $value := . }}
    {{ $key }}: {{ $value }}
  {{ end }}
{{ end }}

spec:
  restartPolicy: {{ .RestartPolicy }}
  hostname: {{ .Hostname }}
  hostNetwork: {{ .HostNetwork }}
  hostAliases:
{{ range .HostAliases }}
  - hostnames:
  {{ range .HostName }}
    - {{ . }}
  {{ end }}
    ip: {{ .IP }}
{{ end }}
  containers:
{{ with .Ctrs }}
  {{ range . }}
  - command:
    {{ range .Cmd }}
    - {{.}}
    {{ end }}
    args:
    {{ range .Arg }}
    - {{.}}
    {{ end }}
    env:
    - name: PATH
      value: /usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
    - name: TERM
      value: xterm
    - name: HOSTNAME
    - name: container
      value: podman
    {{ range .Env }}
    - name: {{ .Name }}
    {{ if (eq .ValueFrom "configmap") }}
      valueFrom:
        configMapKeyRef:
          name: {{ .RefName }}
          key: {{ .RefKey }}
          optional: {{ .Optional }}
    {{ end }}
    {{ if (eq .ValueFrom "secret") }}
      valueFrom:
        secretKeyRef:
          name: {{ .RefName }}
          key: {{ .RefKey }}
          optional: {{ .Optional }}
    {{ end }}
    {{ if (eq .ValueFrom "") }}
      value: {{ .Value }}
    {{ end }}
    {{ end }}
    {{ with .EnvFrom}}
    envFrom:
    {{ range . }}
    {{ if (eq .From "configmap") }}
    - configMapRef:
        name: {{ .Name }}
        optional: {{ .Optional }}
    {{ end }}
    {{ if (eq .From "secret") }}
    - secretRef:
        name: {{ .Name }}
        optional: {{ .Optional }}
    {{ end }}
    {{ end }}
    {{ end }}
    image: {{ .Image }}
    name: {{ .Name }}
    imagePullPolicy: {{ .PullPolicy }}
    {{- if or .CpuRequest .CpuLimit .MemoryRequest .MemoryLimit }}
    resources:
      {{- if or .CpuRequest .MemoryRequest }}
      requests:
        {{if .CpuRequest }}cpu: {{ .CpuRequest }}{{ end }}
        {{if .MemoryRequest }}memory: {{ .MemoryRequest }}{{ end }}
      {{- end }}
      {{- if or .CpuLimit .MemoryLimit }}
      limits:
        {{if .CpuLimit }}cpu: {{ .CpuLimit }}{{ end }}
        {{if .MemoryLimit }}memory: {{ .MemoryLimit }}{{ end }}
      {{- end }}
    {{- end }}
    {{ if .SecurityContext }}
    securityContext:
      allowPrivilegeEscalation: true
      {{ if .Caps }}
      capabilities:
        {{ with .CapAdd }}
        add:
          {{ range . }}
          - {{.}}
          {{ end }}
        {{ end }}
        {{ with .CapDrop }}
        drop:
          {{ range . }}
          - {{.}}
          {{ end }}
        {{ end }}
      {{ end }}
      privileged: false
      readOnlyRootFilesystem: false
    ports:
    - containerPort: {{ .Port }}
      hostIP: {{ .HostIP }}
      hostPort: {{ .Port }}
      protocol: TCP
    workingDir: /
    volumeMounts:
    {{ if .VolumeMount }}
    - name: {{.VolumeName}}
      mountPath: {{ .VolumeMountPath }}
      readonly: {{.VolumeReadOnly}}
      {{ end }}
    {{ end }}
  {{ end }}
{{ end }}
{{ with .Volumes }}
  volumes:
  {{ range . }}
  - name: {{ .Name }}
    {{- if (eq .VolumeType "HostPath") }}
    hostPath:
      path: {{ .HostPath.Path }}
      type: {{ .HostPath.Type }}
    {{- end }}
    {{- if (eq .VolumeType "PersistentVolumeClaim") }}
    persistentVolumeClaim:
      claimName: {{ .PersistentVolumeClaim.ClaimName }}
    {{- end }}
  {{ end }}
{{ end }}
status: {}
`

var deploymentYamlTemplate = `
apiVersion: v1
kind: Deployment
metadata:
  creationTimestamp: "2019-07-17T14:44:08Z"
  name: {{ .Name }}
  labels:
    app: {{ .Name }}
{{ with .Labels }}
  {{ range $key, $value := . }}
    {{ $key }}: {{ $value }}
  {{ end }}
{{ end }}
{{ with .Annotations }}
  annotations:
  {{ range $key, $value := . }}
    {{ $key }}: {{ $value }}
  {{ end }}
{{ end }}

spec:
  replicas: {{ .Replicas }}
  selector:
    matchLabels:
      app: {{ .Name }}
  template:
  {{ with .PodTemplate }}
    metadata:
      labels:
        app: {{ .Name }}
        {{- with .Labels }}{{ range $key, $value := . }}
        {{ $key }}: {{ $value }}
        {{- end }}{{ end }}
      {{- with .Annotations }}
      annotations:
      {{- range $key, $value := . }}
        {{ $key }}: {{ $value }}
      {{- end }}
      {{- end }}
    spec:
      restartPolicy: {{ .RestartPolicy }}
      hostname: {{ .Hostname }}
      hostNetwork: {{ .HostNetwork }}
      containers:
    {{ with .Ctrs }}
      {{ range . }}
      - command:
        {{ range .Cmd }}
        - {{.}}
        {{ end }}
        args:
        {{ range .Arg }}
        - {{.}}
        {{ end }}
        env:
        - name: PATH
          value: /usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
        - name: TERM
          value: xterm
        - name: HOSTNAME
        - name: container
          value: podman
        image: {{ .Image }}
        name: {{ .Name }}
        imagePullPolicy: {{ .PullPolicy }}
        {{- if or .CpuRequest .CpuLimit .MemoryRequest .MemoryLimit }}
        resources:
          {{- if or .CpuRequest .MemoryRequest }}
          requests:
            {{if .CpuRequest }}cpu: {{ .CpuRequest }}{{ end }}
            {{if .MemoryRequest }}memory: {{ .MemoryRequest }}{{ end }}
          {{- end }}
          {{- if or .CpuLimit .MemoryLimit }}
          limits:
            {{if .CpuLimit }}cpu: {{ .CpuLimit }}{{ end }}
            {{if .MemoryLimit }}memory: {{ .MemoryLimit }}{{ end }}
          {{- end }}
        {{- end }}
        {{ if .SecurityContext }}
        securityContext:
          allowPrivilegeEscalation: true
          {{ if .Caps }}
          capabilities:
            {{ with .CapAdd }}
            add:
              {{ range . }}
              - {{.}}
              {{ end }}
            {{ end }}
            {{ with .CapDrop }}
            drop:
              {{ range . }}
              - {{.}}
              {{ end }}
            {{ end }}
          {{ end }}
          privileged: false
          readOnlyRootFilesystem: false
        workingDir: /
        volumeMounts:
        {{ if .VolumeMount }}
        - name: {{.VolumeName}}
          mountPath: {{ .VolumeMountPath }}
          readonly: {{.VolumeReadOnly}}
        {{ end }}
        {{ end }}
      {{ end }}
    {{ end }}
    {{ with .Volumes }}
      volumes:
      {{ range . }}
      - name: {{ .Name }}
        {{- if (eq .VolumeType "HostPath") }}
        hostPath:
          path: {{ .HostPath.Path }}
          type: {{ .HostPath.Type }}
        {{- end }}
        {{- if (eq .VolumeType "PersistentVolumeClaim") }}
        persistentVolumeClaim:
          claimName: {{ .PersistentVolumeClaim.ClaimName }}
        {{- end }}
      {{ end }}
    {{ end }}
{{ end }}
`

var (
	defaultCtrName        = "testCtr"
	defaultCtrCmd         = []string{"top"}
	defaultCtrArg         = []string{"-d", "1.5"}
	defaultCtrImage       = ALPINE
	defaultPodName        = "testPod"
	defaultVolName        = "testVol"
	defaultDeploymentName = "testDeployment"
	defaultConfigMapName  = "testConfigMap"
	defaultPVCName        = "testPVC"
	seccompPwdEPERM       = []byte(`{"defaultAction":"SCMP_ACT_ALLOW","syscalls":[{"name":"getcwd","action":"SCMP_ACT_ERRNO"}]}`)
	// CPU Period in ms
	defaultCPUPeriod = 100
	// Default secret in JSON. Note that the values ("foo" and "bar") are base64 encoded.
	defaultSecret = []byte(`{"FOO":"Zm9v","BAR":"YmFy"}`)
)

func writeYaml(content string, fileName string) error {
	f, err := os.Create(fileName)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.WriteString(content)
	if err != nil {
		return err
	}

	return nil
}

// getKubeYaml returns a kubernetes YAML document.
func getKubeYaml(kind string, object interface{}) (string, error) {
	var yamlTemplate string
	templateBytes := &bytes.Buffer{}

	switch kind {
	case "configmap":
		yamlTemplate = configMapYamlTemplate
	case "pod":
		yamlTemplate = podYamlTemplate
	case "deployment":
		yamlTemplate = deploymentYamlTemplate
	case "persistentVolumeClaim":
		yamlTemplate = persistentVolumeClaimYamlTemplate
	default:
		return "", fmt.Errorf("unsupported kubernetes kind")
	}

	t, err := template.New(kind).Parse(yamlTemplate)
	if err != nil {
		return "", err
	}

	if err := t.Execute(templateBytes, object); err != nil {
		return "", err
	}

	return templateBytes.String(), nil
}

// generateKubeYaml writes a kubernetes YAML document.
func generateKubeYaml(kind string, object interface{}, pathname string) error {
	k, err := getKubeYaml(kind, object)
	if err != nil {
		return err
	}

	return writeYaml(k, pathname)
}

// generateMultiDocKubeYaml writes multiple kube objects in one Yaml document.
func generateMultiDocKubeYaml(kubeObjects []string, pathname string) error {
	var multiKube string

	for _, k := range kubeObjects {
		multiKube += "---\n"
		multiKube += k
	}

	return writeYaml(multiKube, pathname)
}

func createSecret(podmanTest *PodmanTestIntegration, name string, value []byte) {
	secretFilePath := filepath.Join(podmanTest.TempDir, "secret")
	err := ioutil.WriteFile(secretFilePath, value, 0755)
	Expect(err).To(BeNil())

	secret := podmanTest.Podman([]string{"secret", "create", name, secretFilePath})
	secret.WaitWithDefaultTimeout()
	Expect(secret.ExitCode()).To(Equal(0))
}

// ConfigMap describes the options a kube yaml can be configured at configmap level
type ConfigMap struct {
	Name string
	Data map[string]string
}

func getConfigMap(options ...configMapOption) *ConfigMap {
	cm := ConfigMap{
		Name: defaultConfigMapName,
		Data: map[string]string{},
	}

	for _, option := range options {
		option(&cm)
	}

	return &cm
}

type configMapOption func(*ConfigMap)

func withConfigMapName(name string) configMapOption {
	return func(configmap *ConfigMap) {
		configmap.Name = name
	}
}

func withConfigMapData(k, v string) configMapOption {
	return func(configmap *ConfigMap) {
		configmap.Data[k] = v
	}
}

// PVC describes the options a kube yaml can be configured at persistent volume claim level
type PVC struct {
	Name        string
	Annotations map[string]string
}

func getPVC(options ...pvcOption) *PVC {
	pvc := PVC{
		Name:        defaultPVCName,
		Annotations: map[string]string{},
	}

	for _, option := range options {
		option(&pvc)
	}

	return &pvc
}

type pvcOption func(*PVC)

func withPVCName(name string) pvcOption {
	return func(pvc *PVC) {
		pvc.Name = name
	}
}

func withPVCAnnotations(k, v string) pvcOption {
	return func(pvc *PVC) {
		pvc.Annotations[k] = v
	}
}

// Pod describes the options a kube yaml can be configured at pod level
type Pod struct {
	Name          string
	RestartPolicy string
	Hostname      string
	HostNetwork   bool
	HostAliases   []HostAlias
	Ctrs          []*Ctr
	Volumes       []*Volume
	Labels        map[string]string
	Annotations   map[string]string
}

type HostAlias struct {
	IP       string
	HostName []string
}

// getPod takes a list of podOptions and returns a pod with sane defaults
// and the configured options
// if no containers are added, it will add the default container
func getPod(options ...podOption) *Pod {
	p := Pod{
		Name:          defaultPodName,
		RestartPolicy: "Never",
		Hostname:      "",
		HostNetwork:   false,
		HostAliases:   nil,
		Ctrs:          make([]*Ctr, 0),
		Volumes:       make([]*Volume, 0),
		Labels:        make(map[string]string),
		Annotations:   make(map[string]string),
	}
	for _, option := range options {
		option(&p)
	}
	if len(p.Ctrs) == 0 {
		p.Ctrs = []*Ctr{getCtr()}
	}
	return &p
}

type podOption func(*Pod)

func withPodName(name string) podOption {
	return func(pod *Pod) {
		pod.Name = name
	}
}

func withHostname(h string) podOption {
	return func(pod *Pod) {
		pod.Hostname = h
	}
}

func withHostAliases(ip string, host []string) podOption {
	return func(pod *Pod) {
		pod.HostAliases = append(pod.HostAliases, HostAlias{
			IP:       ip,
			HostName: host,
		})
	}
}

func withCtr(c *Ctr) podOption {
	return func(pod *Pod) {
		pod.Ctrs = append(pod.Ctrs, c)
	}
}

func withRestartPolicy(policy string) podOption {
	return func(pod *Pod) {
		pod.RestartPolicy = policy
	}
}

func withLabel(k, v string) podOption {
	return func(pod *Pod) {
		pod.Labels[k] = v
	}
}

func withAnnotation(k, v string) podOption {
	return func(pod *Pod) {
		pod.Annotations[k] = v
	}
}

func withVolume(v *Volume) podOption {
	return func(pod *Pod) {
		pod.Volumes = append(pod.Volumes, v)
	}
}

func withHostNetwork() podOption {
	return func(pod *Pod) {
		pod.HostNetwork = true
	}
}

// Deployment describes the options a kube yaml can be configured at deployment level
type Deployment struct {
	Name        string
	Replicas    int32
	Labels      map[string]string
	Annotations map[string]string
	PodTemplate *Pod
}

func getDeployment(options ...deploymentOption) *Deployment {
	d := Deployment{
		Name:        defaultDeploymentName,
		Replicas:    1,
		Labels:      make(map[string]string),
		Annotations: make(map[string]string),
		PodTemplate: getPod(),
	}
	for _, option := range options {
		option(&d)
	}

	return &d
}

type deploymentOption func(*Deployment)

func withDeploymentLabel(k, v string) deploymentOption {
	return func(deployment *Deployment) {
		deployment.Labels[k] = v
	}
}

func withDeploymentAnnotation(k, v string) deploymentOption {
	return func(deployment *Deployment) {
		deployment.Annotations[k] = v
	}
}

func withPod(pod *Pod) deploymentOption {
	return func(d *Deployment) {
		d.PodTemplate = pod
	}
}

func withReplicas(replicas int32) deploymentOption {
	return func(d *Deployment) {
		d.Replicas = replicas
	}
}

// getPodNamesInDeployment returns list of Pod objects
// with just their name set, so that it can be passed around
// and into getCtrNameInPod for ease of testing
func getPodNamesInDeployment(d *Deployment) []Pod {
	var pods []Pod
	var i int32

	for i = 0; i < d.Replicas; i++ {
		p := Pod{}
		p.Name = fmt.Sprintf("%s-pod-%d", d.Name, i)
		pods = append(pods, p)
	}

	return pods
}

// Ctr describes the options a kube yaml can be configured at container level
type Ctr struct {
	Name            string
	Image           string
	Cmd             []string
	Arg             []string
	CpuRequest      string
	CpuLimit        string
	MemoryRequest   string
	MemoryLimit     string
	SecurityContext bool
	Caps            bool
	CapAdd          []string
	CapDrop         []string
	PullPolicy      string
	HostIP          string
	Port            string
	VolumeMount     bool
	VolumeMountPath string
	VolumeName      string
	VolumeReadOnly  bool
	Env             []Env
	EnvFrom         []EnvFrom
}

// getCtr takes a list of ctrOptions and returns a Ctr with sane defaults
// and the configured options
func getCtr(options ...ctrOption) *Ctr {
	c := Ctr{
		Name:            defaultCtrName,
		Image:           defaultCtrImage,
		Cmd:             defaultCtrCmd,
		Arg:             defaultCtrArg,
		SecurityContext: true,
		Caps:            false,
		CapAdd:          nil,
		CapDrop:         nil,
		PullPolicy:      "",
		HostIP:          "",
		Port:            "",
		VolumeMount:     false,
		VolumeMountPath: "",
		VolumeName:      "",
		VolumeReadOnly:  false,
		Env:             []Env{},
		EnvFrom:         []EnvFrom{},
	}
	for _, option := range options {
		option(&c)
	}
	return &c
}

type ctrOption func(*Ctr)

func withCmd(cmd []string) ctrOption {
	return func(c *Ctr) {
		c.Cmd = cmd
	}
}

func withArg(arg []string) ctrOption {
	return func(c *Ctr) {
		c.Arg = arg
	}
}

func withImage(img string) ctrOption {
	return func(c *Ctr) {
		c.Image = img
	}
}

func withCpuRequest(request string) ctrOption {
	return func(c *Ctr) {
		c.CpuRequest = request
	}
}

func withCpuLimit(limit string) ctrOption {
	return func(c *Ctr) {
		c.CpuLimit = limit
	}
}

func withMemoryRequest(request string) ctrOption {
	return func(c *Ctr) {
		c.MemoryRequest = request
	}
}

func withMemoryLimit(limit string) ctrOption {
	return func(c *Ctr) {
		c.MemoryLimit = limit
	}
}

func withSecurityContext(sc bool) ctrOption {
	return func(c *Ctr) {
		c.SecurityContext = sc
	}
}

func withCapAdd(caps []string) ctrOption {
	return func(c *Ctr) {
		c.CapAdd = caps
		c.Caps = true
	}
}

func withCapDrop(caps []string) ctrOption {
	return func(c *Ctr) {
		c.CapDrop = caps
		c.Caps = true
	}
}

func withPullPolicy(policy string) ctrOption {
	return func(c *Ctr) {
		c.PullPolicy = policy
	}
}

func withHostIP(ip string, port string) ctrOption {
	return func(c *Ctr) {
		c.HostIP = ip
		c.Port = port
	}
}

func withVolumeMount(mountPath string, readonly bool) ctrOption {
	return func(c *Ctr) {
		c.VolumeMountPath = mountPath
		c.VolumeName = defaultVolName
		c.VolumeReadOnly = readonly
		c.VolumeMount = true
	}
}

func withEnv(name, value, valueFrom, refName, refKey string, optional bool) ctrOption {
	return func(c *Ctr) {
		e := Env{
			Name:      name,
			Value:     value,
			ValueFrom: valueFrom,
			RefName:   refName,
			RefKey:    refKey,
			Optional:  optional,
		}

		c.Env = append(c.Env, e)
	}
}

func withEnvFrom(name, from string, optional bool) ctrOption {
	return func(c *Ctr) {
		e := EnvFrom{
			Name:     name,
			From:     from,
			Optional: optional,
		}

		c.EnvFrom = append(c.EnvFrom, e)
	}
}

func getCtrNameInPod(pod *Pod) string {
	return fmt.Sprintf("%s-%s", pod.Name, defaultCtrName)
}

type HostPath struct {
	Path string
	Type string
}

type PersistentVolumeClaim struct {
	ClaimName string
}

type Volume struct {
	VolumeType string
	Name       string
	HostPath
	PersistentVolumeClaim
}

// getHostPathVolume takes a type and a location for a HostPath
// volume giving it a default name of volName
func getHostPathVolume(vType, vPath string) *Volume {
	return &Volume{
		VolumeType: "HostPath",
		Name:       defaultVolName,
		HostPath: HostPath{
			Path: vPath,
			Type: vType,
		},
	}
}

// getHostPathVolume takes a name for a Persistentvolumeclaim
// volume giving it a default name of volName
func getPersistentVolumeClaimVolume(vName string) *Volume {
	return &Volume{
		VolumeType: "PersistentVolumeClaim",
		Name:       defaultVolName,
		PersistentVolumeClaim: PersistentVolumeClaim{
			ClaimName: vName,
		},
	}
}

type Env struct {
	Name      string
	Value     string
	ValueFrom string
	RefName   string
	RefKey    string
	Optional  bool
}

type EnvFrom struct {
	Name     string
	From     string
	Optional bool
}

func milliCPUToQuota(milliCPU string) int {
	milli, _ := strconv.Atoi(strings.Trim(milliCPU, "m"))
	return milli * defaultCPUPeriod
}

var _ = Describe("Podman play kube", func() {
	var (
		tempdir    string
		err        error
		podmanTest *PodmanTestIntegration
		kubeYaml   string
	)

	BeforeEach(func() {
		tempdir, err = CreateTempDirInTempDir()
		if err != nil {
			os.Exit(1)
		}
		podmanTest = PodmanTestCreate(tempdir)
		podmanTest.Setup()
		podmanTest.SeedImages()

		kubeYaml = filepath.Join(podmanTest.TempDir, "kube.yaml")
	})

	AfterEach(func() {
		podmanTest.Cleanup()
		f := CurrentGinkgoTestDescription()
		processTestResult(f)
	})

	It("podman play kube fail with yaml of unsupported kind", func() {
		err := writeYaml(unknownKindYaml, kubeYaml)
		Expect(err).To(BeNil())

		kube := podmanTest.Podman([]string{"play", "kube", kubeYaml})
		kube.WaitWithDefaultTimeout()
		Expect(kube.ExitCode()).To(Not(Equal(0)))

	})

	It("podman play kube fail with custom selinux label", func() {
		if !selinux.GetEnabled() {
			Skip("SELinux not enabled")
		}
		err := writeYaml(selinuxLabelPodYaml, kubeYaml)
		Expect(err).To(BeNil())

		kube := podmanTest.Podman([]string{"play", "kube", kubeYaml})
		kube.WaitWithDefaultTimeout()
		Expect(kube.ExitCode()).To(Equal(0))

		inspect := podmanTest.Podman([]string{"inspect", "label-pod-test", "--format", "'{{ .ProcessLabel }}'"})
		inspect.WaitWithDefaultTimeout()
		label := inspect.OutputToString()

		Expect(label).To(ContainSubstring("unconfined_u:system_r:spc_t:s0"))
	})

	It("podman play kube fail with nonexistent authfile", func() {
		err := generateKubeYaml("pod", getPod(), kubeYaml)
		Expect(err).To(BeNil())

		kube := podmanTest.Podman([]string{"play", "kube", "--authfile", "/tmp/nonexistent", kubeYaml})
		kube.WaitWithDefaultTimeout()
		Expect(kube.ExitCode()).To(Not(Equal(0)))

	})

	It("podman play kube test correct command", func() {
		pod := getPod()
		err := generateKubeYaml("pod", pod, kubeYaml)
		Expect(err).To(BeNil())

		kube := podmanTest.Podman([]string{"play", "kube", kubeYaml})
		kube.WaitWithDefaultTimeout()
		Expect(kube.ExitCode()).To(Equal(0))

		inspect := podmanTest.Podman([]string{"inspect", getCtrNameInPod(pod), "--format", "'{{ .Config.Cmd }}'"})
		inspect.WaitWithDefaultTimeout()
		Expect(inspect.ExitCode()).To(Equal(0))
		cmd := inspect.OutputToString()

		inspect = podmanTest.Podman([]string{"inspect", getCtrNameInPod(pod), "--format", "'{{ .Config.Entrypoint }}'"})
		inspect.WaitWithDefaultTimeout()
		Expect(inspect.ExitCode()).To(Equal(0))
		ep := inspect.OutputToString()

		// Use the defined command to override the image's command
		Expect(ep).To(ContainSubstring(strings.Join(defaultCtrCmd, " ")))
		Expect(cmd).To(ContainSubstring(strings.Join(defaultCtrArg, " ")))
	})

	// If you do not supply command or args for a Container, the defaults defined in the Docker image are used.
	It("podman play kube test correct args and cmd when not specified", func() {
		pod := getPod(withCtr(getCtr(withImage(registry), withCmd(nil), withArg(nil))))
		err := generateKubeYaml("pod", pod, kubeYaml)
		Expect(err).To(BeNil())

		kube := podmanTest.Podman([]string{"play", "kube", kubeYaml})
		kube.WaitWithDefaultTimeout()
		Expect(kube.ExitCode()).To(Equal(0))

		// this image's ENTRYPOINT is `/entrypoint.sh`
		inspect := podmanTest.Podman([]string{"inspect", getCtrNameInPod(pod), "--format", "'{{ .Config.Entrypoint }}'"})
		inspect.WaitWithDefaultTimeout()
		Expect(inspect.ExitCode()).To(Equal(0))
		Expect(inspect.OutputToString()).To(ContainSubstring(`/entrypoint.sh`))

		// and its COMMAND is `/etc/docker/registry/config.yml`
		inspect = podmanTest.Podman([]string{"inspect", getCtrNameInPod(pod), "--format", "'{{ .Config.Cmd }}'"})
		inspect.WaitWithDefaultTimeout()
		Expect(inspect.ExitCode()).To(Equal(0))
		Expect(inspect.OutputToString()).To(ContainSubstring(`[/etc/docker/registry/config.yml]`))
	})

	// If you supply a command but no args for a Container, only the supplied command is used.
	// The default EntryPoint and the default Cmd defined in the Docker image are ignored.
	It("podman play kube test correct command with only set command in yaml file", func() {
		pod := getPod(withCtr(getCtr(withImage(registry), withCmd([]string{"echo", "hello"}), withArg(nil))))
		err := generateKubeYaml("pod", pod, kubeYaml)
		Expect(err).To(BeNil())

		kube := podmanTest.Podman([]string{"play", "kube", kubeYaml})
		kube.WaitWithDefaultTimeout()
		Expect(kube.ExitCode()).To(Equal(0))

		// Use the defined command to override the image's command, and don't set the args
		// so the full command in result should not contains the image's command
		inspect := podmanTest.Podman([]string{"inspect", getCtrNameInPod(pod), "--format", "'{{ .Config.Entrypoint }}'"})
		inspect.WaitWithDefaultTimeout()
		Expect(inspect.ExitCode()).To(Equal(0))
		Expect(inspect.OutputToString()).To(ContainSubstring(`echo hello`))

		inspect = podmanTest.Podman([]string{"inspect", getCtrNameInPod(pod), "--format", "'{{ .Config.Cmd }}'"})
		inspect.WaitWithDefaultTimeout()
		Expect(inspect.ExitCode()).To(Equal(0))
		// an empty command is reported as '[]'
		Expect(inspect.OutputToString()).To(ContainSubstring(`[]`))
	})

	// If you supply only args for a Container, the default Entrypoint defined in the Docker image is run with the args that you supplied.
	It("podman play kube test correct command with only set args in yaml file", func() {
		pod := getPod(withCtr(getCtr(withImage(registry), withCmd(nil), withArg([]string{"echo", "hello"}))))
		err := generateKubeYaml("pod", pod, kubeYaml)
		Expect(err).To(BeNil())

		kube := podmanTest.Podman([]string{"play", "kube", kubeYaml})
		kube.WaitWithDefaultTimeout()
		Expect(kube.ExitCode()).To(Equal(0))

		// this image's ENTRYPOINT is `/entrypoint.sh`
		inspect := podmanTest.Podman([]string{"inspect", getCtrNameInPod(pod), "--format", "'{{ .Config.Entrypoint }}'"})
		inspect.WaitWithDefaultTimeout()
		Expect(inspect.ExitCode()).To(Equal(0))
		Expect(inspect.OutputToString()).To(ContainSubstring(`/entrypoint.sh`))

		inspect = podmanTest.Podman([]string{"inspect", getCtrNameInPod(pod), "--format", "'{{ .Config.Cmd }}'"})
		inspect.WaitWithDefaultTimeout()
		Expect(inspect.ExitCode()).To(Equal(0))
		Expect(inspect.OutputToString()).To(ContainSubstring(`[echo hello]`))
	})

	// If you supply a command and args,
	// the default Entrypoint and the default Cmd defined in the Docker image are ignored.
	// Your command is run with your args.
	It("podman play kube test correct command with both set args and cmd in yaml file", func() {
		pod := getPod(withCtr(getCtr(withImage(registry), withCmd([]string{"echo"}), withArg([]string{"hello"}))))
		err := generateKubeYaml("pod", pod, kubeYaml)
		Expect(err).To(BeNil())

		kube := podmanTest.Podman([]string{"play", "kube", kubeYaml})
		kube.WaitWithDefaultTimeout()
		Expect(kube.ExitCode()).To(Equal(0))

		inspect := podmanTest.Podman([]string{"inspect", getCtrNameInPod(pod), "--format", "'{{ .Config.Entrypoint }}'"})
		inspect.WaitWithDefaultTimeout()
		Expect(inspect.ExitCode()).To(Equal(0))
		Expect(inspect.OutputToString()).To(ContainSubstring(`echo`))

		inspect = podmanTest.Podman([]string{"inspect", getCtrNameInPod(pod), "--format", "'{{ .Config.Cmd }}'"})
		inspect.WaitWithDefaultTimeout()
		Expect(inspect.ExitCode()).To(Equal(0))
		Expect(inspect.OutputToString()).To(ContainSubstring(`[hello]`))
	})

	It("podman play kube test correct output", func() {
		p := getPod(withCtr(getCtr(withCmd([]string{"echo", "hello"}), withArg([]string{"world"}))))

		err := generateKubeYaml("pod", p, kubeYaml)
		Expect(err).To(BeNil())

		kube := podmanTest.Podman([]string{"play", "kube", kubeYaml})
		kube.WaitWithDefaultTimeout()
		Expect(kube.ExitCode()).To(Equal(0))

		logs := podmanTest.Podman([]string{"logs", getCtrNameInPod(p)})
		logs.WaitWithDefaultTimeout()
		Expect(logs.ExitCode()).To(Equal(0))
		Expect(logs.OutputToString()).To(ContainSubstring("hello world"))
	})

	It("podman play kube test restartPolicy", func() {
		// podName,  set,  expect
		testSli := [][]string{
			{"testPod1", "", "always"}, // Default equal to always
			{"testPod2", "Always", "always"},
			{"testPod3", "OnFailure", "on-failure"},
			{"testPod4", "Never", "no"},
		}
		for _, v := range testSli {
			pod := getPod(withPodName(v[0]), withRestartPolicy(v[1]))
			err := generateKubeYaml("pod", pod, kubeYaml)
			Expect(err).To(BeNil())

			kube := podmanTest.Podman([]string{"play", "kube", kubeYaml})
			kube.WaitWithDefaultTimeout()
			Expect(kube.ExitCode()).To(Equal(0))

			inspect := podmanTest.Podman([]string{"inspect", getCtrNameInPod(pod), "--format", "{{.HostConfig.RestartPolicy.Name}}"})
			inspect.WaitWithDefaultTimeout()
			Expect(inspect.ExitCode()).To(Equal(0))
			Expect(inspect.OutputToString()).To(Equal(v[2]))
		}
	})

	It("podman play kube test env value from configmap", func() {
		SkipIfRemote("configmap list is not supported as a param")
		cmYamlPathname := filepath.Join(podmanTest.TempDir, "foo-cm.yaml")
		cm := getConfigMap(withConfigMapName("foo"), withConfigMapData("FOO", "foo"))
		err := generateKubeYaml("configmap", cm, cmYamlPathname)
		Expect(err).To(BeNil())

		pod := getPod(withCtr(getCtr(withEnv("FOO", "", "configmap", "foo", "FOO", false))))
		err = generateKubeYaml("pod", pod, kubeYaml)
		Expect(err).To(BeNil())

		kube := podmanTest.Podman([]string{"play", "kube", kubeYaml, "--configmap", cmYamlPathname})
		kube.WaitWithDefaultTimeout()
		Expect(kube.ExitCode()).To(Equal(0))

		inspect := podmanTest.Podman([]string{"inspect", getCtrNameInPod(pod), "--format", "'{{ .Config.Env }}'"})
		inspect.WaitWithDefaultTimeout()
		Expect(inspect.ExitCode()).To(Equal(0))
		Expect(inspect.OutputToString()).To(ContainSubstring(`FOO=foo`))
	})

	It("podman play kube test required env value from configmap with missing key", func() {
		SkipIfRemote("configmap list is not supported as a param")
		cmYamlPathname := filepath.Join(podmanTest.TempDir, "foo-cm.yaml")
		cm := getConfigMap(withConfigMapName("foo"), withConfigMapData("FOO", "foo"))
		err := generateKubeYaml("configmap", cm, cmYamlPathname)
		Expect(err).To(BeNil())

		pod := getPod(withCtr(getCtr(withEnv("FOO", "", "configmap", "foo", "MISSING_KEY", false))))
		err = generateKubeYaml("pod", pod, kubeYaml)
		Expect(err).To(BeNil())

		kube := podmanTest.Podman([]string{"play", "kube", kubeYaml, "--configmap", cmYamlPathname})
		kube.WaitWithDefaultTimeout()
		Expect(kube.ExitCode()).To(Not(Equal(0)))
	})

	It("podman play kube test required env value from missing configmap", func() {
		pod := getPod(withCtr(getCtr(withEnv("FOO", "", "configmap", "missing_cm", "FOO", false))))
		err = generateKubeYaml("pod", pod, kubeYaml)
		Expect(err).To(BeNil())

		kube := podmanTest.Podman([]string{"play", "kube", kubeYaml})
		kube.WaitWithDefaultTimeout()
		Expect(kube.ExitCode()).To(Not(Equal(0)))
	})

	It("podman play kube test optional env value from configmap with missing key", func() {
		SkipIfRemote("configmap list is not supported as a param")
		cmYamlPathname := filepath.Join(podmanTest.TempDir, "foo-cm.yaml")
		cm := getConfigMap(withConfigMapName("foo"), withConfigMapData("FOO", "foo"))
		err := generateKubeYaml("configmap", cm, cmYamlPathname)
		Expect(err).To(BeNil())

		pod := getPod(withCtr(getCtr(withEnv("FOO", "", "configmap", "foo", "MISSING_KEY", true))))
		err = generateKubeYaml("pod", pod, kubeYaml)
		Expect(err).To(BeNil())

		kube := podmanTest.Podman([]string{"play", "kube", kubeYaml, "--configmap", cmYamlPathname})
		kube.WaitWithDefaultTimeout()
		Expect(kube.ExitCode()).To(Equal(0))

		inspect := podmanTest.Podman([]string{"inspect", getCtrNameInPod(pod), "--format", "'{{ range .Config.Env }}[{{ . }}]{{end}}'"})
		inspect.WaitWithDefaultTimeout()
		Expect(inspect.ExitCode()).To(Equal(0))
		Expect(inspect.OutputToString()).To(ContainSubstring(`[FOO=]`))
	})

	It("podman play kube test optional env value from missing configmap", func() {
		pod := getPod(withCtr(getCtr(withEnv("FOO", "", "configmap", "missing_cm", "FOO", true))))
		err = generateKubeYaml("pod", pod, kubeYaml)
		Expect(err).To(BeNil())

		kube := podmanTest.Podman([]string{"play", "kube", kubeYaml})
		kube.WaitWithDefaultTimeout()
		Expect(kube.ExitCode()).To(Equal(0))

		inspect := podmanTest.Podman([]string{"inspect", getCtrNameInPod(pod), "--format", "'{{ range .Config.Env }}[{{ . }}]{{end}}'"})
		inspect.WaitWithDefaultTimeout()
		Expect(inspect.ExitCode()).To(Equal(0))
		Expect(inspect.OutputToString()).To(ContainSubstring(`[FOO=]`))
	})

	It("podman play kube test get all key-value pairs from configmap as envs", func() {
		SkipIfRemote("configmap list is not supported as a param")
		cmYamlPathname := filepath.Join(podmanTest.TempDir, "foo-cm.yaml")
		cm := getConfigMap(withConfigMapName("foo"), withConfigMapData("FOO1", "foo1"), withConfigMapData("FOO2", "foo2"))
		err := generateKubeYaml("configmap", cm, cmYamlPathname)
		Expect(err).To(BeNil())

		pod := getPod(withCtr(getCtr(withEnvFrom("foo", "configmap", false))))
		err = generateKubeYaml("pod", pod, kubeYaml)
		Expect(err).To(BeNil())

		kube := podmanTest.Podman([]string{"play", "kube", kubeYaml, "--configmap", cmYamlPathname})
		kube.WaitWithDefaultTimeout()
		Expect(kube.ExitCode()).To(Equal(0))

		inspect := podmanTest.Podman([]string{"inspect", getCtrNameInPod(pod), "--format", "'{{ .Config.Env }}'"})
		inspect.WaitWithDefaultTimeout()
		Expect(inspect.ExitCode()).To(Equal(0))
		Expect(inspect.OutputToString()).To(ContainSubstring(`FOO1=foo1`))
		Expect(inspect.OutputToString()).To(ContainSubstring(`FOO2=foo2`))
	})

	It("podman play kube test get all key-value pairs from required configmap as envs", func() {
		pod := getPod(withCtr(getCtr(withEnvFrom("missing_cm", "configmap", false))))
		err = generateKubeYaml("pod", pod, kubeYaml)
		Expect(err).To(BeNil())

		kube := podmanTest.Podman([]string{"play", "kube", kubeYaml})
		kube.WaitWithDefaultTimeout()
		Expect(kube.ExitCode()).To(Not(Equal(0)))
	})

	It("podman play kube test get all key-value pairs from optional configmap as envs", func() {
		pod := getPod(withCtr(getCtr(withEnvFrom("missing_cm", "configmap", true))))
		err = generateKubeYaml("pod", pod, kubeYaml)
		Expect(err).To(BeNil())

		kube := podmanTest.Podman([]string{"play", "kube", kubeYaml})
		kube.WaitWithDefaultTimeout()
		Expect(kube.ExitCode()).To(Equal(0))
	})

	It("podman play kube test env value from secret", func() {
		createSecret(podmanTest, "foo", defaultSecret)
		pod := getPod(withCtr(getCtr(withEnv("FOO", "", "secret", "foo", "FOO", false))))
		err = generateKubeYaml("pod", pod, kubeYaml)
		Expect(err).To(BeNil())

		kube := podmanTest.Podman([]string{"play", "kube", kubeYaml})
		kube.WaitWithDefaultTimeout()
		Expect(kube.ExitCode()).To(Equal(0))

		inspect := podmanTest.Podman([]string{"inspect", getCtrNameInPod(pod), "--format", "'{{ .Config.Env }}'"})
		inspect.WaitWithDefaultTimeout()
		Expect(inspect.ExitCode()).To(Equal(0))
		Expect(inspect.OutputToString()).To(ContainSubstring(`FOO=foo`))
	})

	It("podman play kube test required env value from missing secret", func() {
		pod := getPod(withCtr(getCtr(withEnv("FOO", "", "secret", "foo", "FOO", false))))
		err = generateKubeYaml("pod", pod, kubeYaml)
		Expect(err).To(BeNil())

		kube := podmanTest.Podman([]string{"play", "kube", kubeYaml})
		kube.WaitWithDefaultTimeout()
		Expect(kube.ExitCode()).To(Not(Equal(0)))
	})

	It("podman play kube test required env value from secret with missing key", func() {
		createSecret(podmanTest, "foo", defaultSecret)
		pod := getPod(withCtr(getCtr(withEnv("FOO", "", "secret", "foo", "MISSING", false))))
		err = generateKubeYaml("pod", pod, kubeYaml)
		Expect(err).To(BeNil())

		kube := podmanTest.Podman([]string{"play", "kube", kubeYaml})
		kube.WaitWithDefaultTimeout()
		Expect(kube.ExitCode()).To(Not(Equal(0)))
	})

	It("podman play kube test optional env value from missing secret", func() {
		pod := getPod(withCtr(getCtr(withEnv("FOO", "", "secret", "foo", "FOO", true))))
		err = generateKubeYaml("pod", pod, kubeYaml)
		Expect(err).To(BeNil())

		kube := podmanTest.Podman([]string{"play", "kube", kubeYaml})
		kube.WaitWithDefaultTimeout()
		Expect(kube.ExitCode()).To(Equal(0))

		inspect := podmanTest.Podman([]string{"inspect", getCtrNameInPod(pod), "--format", "'{{ range .Config.Env }}[{{ . }}]{{end}}'"})
		inspect.WaitWithDefaultTimeout()
		Expect(inspect.ExitCode()).To(Equal(0))
		Expect(inspect.OutputToString()).To(ContainSubstring(`[FOO=]`))
	})

	It("podman play kube test optional env value from secret with missing key", func() {
		createSecret(podmanTest, "foo", defaultSecret)
		pod := getPod(withCtr(getCtr(withEnv("FOO", "", "secret", "foo", "MISSING", true))))
		err = generateKubeYaml("pod", pod, kubeYaml)
		Expect(err).To(BeNil())

		kube := podmanTest.Podman([]string{"play", "kube", kubeYaml})
		kube.WaitWithDefaultTimeout()
		Expect(kube.ExitCode()).To(Equal(0))

		inspect := podmanTest.Podman([]string{"inspect", getCtrNameInPod(pod), "--format", "'{{ range .Config.Env }}[{{ . }}]{{end}}'"})
		inspect.WaitWithDefaultTimeout()
		Expect(inspect.ExitCode()).To(Equal(0))
		Expect(inspect.OutputToString()).To(ContainSubstring(`[FOO=]`))
	})

	It("podman play kube test get all key-value pairs from secret as envs", func() {
		createSecret(podmanTest, "foo", defaultSecret)
		pod := getPod(withCtr(getCtr(withEnvFrom("foo", "secret", false))))
		err = generateKubeYaml("pod", pod, kubeYaml)
		Expect(err).To(BeNil())

		kube := podmanTest.Podman([]string{"play", "kube", kubeYaml})
		kube.WaitWithDefaultTimeout()
		Expect(kube.ExitCode()).To(Equal(0))

		inspect := podmanTest.Podman([]string{"inspect", getCtrNameInPod(pod), "--format", "'{{ .Config.Env }}'"})
		inspect.WaitWithDefaultTimeout()
		Expect(inspect.ExitCode()).To(Equal(0))
		Expect(inspect.OutputToString()).To(ContainSubstring(`FOO=foo`))
		Expect(inspect.OutputToString()).To(ContainSubstring(`BAR=bar`))
	})

	It("podman play kube test get all key-value pairs from required secret as envs", func() {
		pod := getPod(withCtr(getCtr(withEnvFrom("missing_secret", "secret", false))))
		err = generateKubeYaml("pod", pod, kubeYaml)
		Expect(err).To(BeNil())

		kube := podmanTest.Podman([]string{"play", "kube", kubeYaml})
		kube.WaitWithDefaultTimeout()
		Expect(kube.ExitCode()).To(Not(Equal(0)))
	})

	It("podman play kube test get all key-value pairs from optional secret as envs", func() {
		pod := getPod(withCtr(getCtr(withEnvFrom("missing_secret", "secret", true))))
		err = generateKubeYaml("pod", pod, kubeYaml)
		Expect(err).To(BeNil())

		kube := podmanTest.Podman([]string{"play", "kube", kubeYaml})
		kube.WaitWithDefaultTimeout()
		Expect(kube.ExitCode()).To(Equal(0))
	})

	It("podman play kube test hostname", func() {
		pod := getPod()
		err := generateKubeYaml("pod", pod, kubeYaml)
		Expect(err).To(BeNil())

		kube := podmanTest.Podman([]string{"play", "kube", kubeYaml})
		kube.WaitWithDefaultTimeout()
		Expect(kube.ExitCode()).To(Equal(0))

		inspect := podmanTest.Podman([]string{"inspect", getCtrNameInPod(pod), "--format", "{{ .Config.Hostname }}"})
		inspect.WaitWithDefaultTimeout()
		Expect(inspect.ExitCode()).To(Equal(0))
		Expect(inspect.OutputToString()).To(Equal(defaultPodName))
	})

	It("podman play kube test with customized hostname", func() {
		hostname := "myhostname"
		pod := getPod(withHostname(hostname))
		err := generateKubeYaml("pod", getPod(withHostname(hostname)), kubeYaml)
		Expect(err).To(BeNil())

		kube := podmanTest.Podman([]string{"play", "kube", kubeYaml})
		kube.WaitWithDefaultTimeout()
		Expect(kube.ExitCode()).To(Equal(0))

		inspect := podmanTest.Podman([]string{"inspect", getCtrNameInPod(pod), "--format", "{{ .Config.Hostname }}"})
		inspect.WaitWithDefaultTimeout()
		Expect(inspect.ExitCode()).To(Equal(0))
		Expect(inspect.OutputToString()).To(Equal(hostname))
	})

	It("podman play kube test HostAliases", func() {
		pod := getPod(withHostAliases("192.168.1.2", []string{
			"test1.podman.io",
			"test2.podman.io",
		}),
			withHostAliases("192.168.1.3", []string{
				"test3.podman.io",
				"test4.podman.io",
			}),
		)
		err := generateKubeYaml("pod", pod, kubeYaml)
		Expect(err).To(BeNil())

		kube := podmanTest.Podman([]string{"play", "kube", kubeYaml})
		kube.WaitWithDefaultTimeout()
		Expect(kube.ExitCode()).To(Equal(0))

		inspect := podmanTest.Podman([]string{"inspect", pod.Name, "--format", "{{ .InfraConfig.HostAdd}}"})
		inspect.WaitWithDefaultTimeout()
		Expect(inspect.ExitCode()).To(Equal(0))
		Expect(inspect.OutputToString()).
			To(Equal("[test1.podman.io:192.168.1.2 test2.podman.io:192.168.1.2 test3.podman.io:192.168.1.3 test4.podman.io:192.168.1.3]"))
	})

	It("podman play kube cap add", func() {
		capAdd := "CAP_SYS_ADMIN"
		ctr := getCtr(withCapAdd([]string{capAdd}), withCmd([]string{"cat", "/proc/self/status"}), withArg(nil))

		pod := getPod(withCtr(ctr))
		err := generateKubeYaml("pod", pod, kubeYaml)
		Expect(err).To(BeNil())

		kube := podmanTest.Podman([]string{"play", "kube", kubeYaml})
		kube.WaitWithDefaultTimeout()
		Expect(kube.ExitCode()).To(Equal(0))

		inspect := podmanTest.Podman([]string{"inspect", getCtrNameInPod(pod)})
		inspect.WaitWithDefaultTimeout()
		Expect(inspect.ExitCode()).To(Equal(0))
		Expect(inspect.OutputToString()).To(ContainSubstring(capAdd))
	})

	It("podman play kube cap drop", func() {
		capDrop := "CAP_CHOWN"
		ctr := getCtr(withCapDrop([]string{capDrop}))

		pod := getPod(withCtr(ctr))
		err := generateKubeYaml("pod", pod, kubeYaml)
		Expect(err).To(BeNil())

		kube := podmanTest.Podman([]string{"play", "kube", kubeYaml})
		kube.WaitWithDefaultTimeout()
		Expect(kube.ExitCode()).To(Equal(0))

		inspect := podmanTest.Podman([]string{"inspect", getCtrNameInPod(pod)})
		inspect.WaitWithDefaultTimeout()
		Expect(inspect.ExitCode()).To(Equal(0))
		Expect(inspect.OutputToString()).To(ContainSubstring(capDrop))
	})

	It("podman play kube no security context", func() {
		// expect play kube to not fail if no security context is specified
		pod := getPod(withCtr(getCtr(withSecurityContext(false))))
		err := generateKubeYaml("pod", pod, kubeYaml)
		Expect(err).To(BeNil())

		kube := podmanTest.Podman([]string{"play", "kube", kubeYaml})
		kube.WaitWithDefaultTimeout()
		Expect(kube.ExitCode()).To(Equal(0))

		inspect := podmanTest.Podman([]string{"inspect", getCtrNameInPod(pod)})
		inspect.WaitWithDefaultTimeout()
		Expect(inspect.ExitCode()).To(Equal(0))
	})

	It("podman play kube seccomp container level", func() {
		SkipIfRemote("podman-remote does not support --seccomp-profile-root flag")
		// expect play kube is expected to set a seccomp label if it's applied as an annotation
		jsonFile, err := podmanTest.CreateSeccompJson(seccompPwdEPERM)
		if err != nil {
			fmt.Println(err)
			Skip("Failed to prepare seccomp.json for test.")
		}

		ctrAnnotation := "container.seccomp.security.alpha.kubernetes.io/" + defaultCtrName
		ctr := getCtr(withCmd([]string{"pwd"}), withArg(nil))

		pod := getPod(withCtr(ctr), withAnnotation(ctrAnnotation, "localhost/"+filepath.Base(jsonFile)))
		err = generateKubeYaml("pod", pod, kubeYaml)
		Expect(err).To(BeNil())

		// CreateSeccompJson will put the profile into podmanTest.TempDir. Use --seccomp-profile-root to tell play kube where to look
		kube := podmanTest.Podman([]string{"play", "kube", "--seccomp-profile-root", podmanTest.TempDir, kubeYaml})
		kube.WaitWithDefaultTimeout()
		Expect(kube.ExitCode()).To(Equal(0))

		logs := podmanTest.Podman([]string{"logs", getCtrNameInPod(pod)})
		logs.WaitWithDefaultTimeout()
		Expect(logs.ExitCode()).To(Equal(0))
		Expect(logs.ErrorToString()).To(ContainSubstring("Operation not permitted"))
	})

	It("podman play kube seccomp pod level", func() {
		SkipIfRemote("podman-remote does not support --seccomp-profile-root flag")
		// expect play kube is expected to set a seccomp label if it's applied as an annotation
		jsonFile, err := podmanTest.CreateSeccompJson(seccompPwdEPERM)
		if err != nil {
			fmt.Println(err)
			Skip("Failed to prepare seccomp.json for test.")
		}
		defer os.Remove(jsonFile)

		ctr := getCtr(withCmd([]string{"pwd"}), withArg(nil))

		pod := getPod(withCtr(ctr), withAnnotation("seccomp.security.alpha.kubernetes.io/pod", "localhost/"+filepath.Base(jsonFile)))
		err = generateKubeYaml("pod", pod, kubeYaml)
		Expect(err).To(BeNil())

		// CreateSeccompJson will put the profile into podmanTest.TempDir. Use --seccomp-profile-root to tell play kube where to look
		kube := podmanTest.Podman([]string{"play", "kube", "--seccomp-profile-root", podmanTest.TempDir, kubeYaml})
		kube.WaitWithDefaultTimeout()
		Expect(kube.ExitCode()).To(Equal(0))

		logs := podmanTest.Podman([]string{"logs", getCtrNameInPod(pod)})
		logs.WaitWithDefaultTimeout()
		Expect(logs.ExitCode()).To(Equal(0))
		Expect(logs.ErrorToString()).To(ContainSubstring("Operation not permitted"))
	})

	It("podman play kube with pull policy of never should be 125", func() {
		ctr := getCtr(withPullPolicy("never"), withImage(BB_GLIBC))
		err := generateKubeYaml("pod", getPod(withCtr(ctr)), kubeYaml)
		Expect(err).To(BeNil())

		kube := podmanTest.Podman([]string{"play", "kube", kubeYaml})
		kube.WaitWithDefaultTimeout()
		Expect(kube.ExitCode()).To(Equal(125))
	})

	It("podman play kube with pull policy of missing", func() {
		ctr := getCtr(withPullPolicy("missing"), withImage(BB))
		err := generateKubeYaml("pod", getPod(withCtr(ctr)), kubeYaml)
		Expect(err).To(BeNil())

		kube := podmanTest.Podman([]string{"play", "kube", kubeYaml})
		kube.WaitWithDefaultTimeout()
		Expect(kube.ExitCode()).To(Equal(0))
	})

	It("podman play kube with pull always", func() {
		oldBB := "quay.io/libpod/busybox:1.30.1"
		pull := podmanTest.Podman([]string{"pull", oldBB})
		pull.WaitWithDefaultTimeout()

		tag := podmanTest.Podman([]string{"tag", oldBB, BB})
		tag.WaitWithDefaultTimeout()
		Expect(tag.ExitCode()).To(BeZero())

		rmi := podmanTest.Podman([]string{"rmi", oldBB})
		rmi.WaitWithDefaultTimeout()
		Expect(rmi.ExitCode()).To(BeZero())

		inspect := podmanTest.Podman([]string{"inspect", BB})
		inspect.WaitWithDefaultTimeout()
		oldBBinspect := inspect.InspectImageJSON()

		ctr := getCtr(withPullPolicy("always"), withImage(BB))
		err := generateKubeYaml("pod", getPod(withCtr(ctr)), kubeYaml)
		Expect(err).To(BeNil())

		kube := podmanTest.Podman([]string{"play", "kube", kubeYaml})
		kube.WaitWithDefaultTimeout()
		Expect(kube.ExitCode()).To(Equal(0))

		inspect = podmanTest.Podman([]string{"inspect", BB})
		inspect.WaitWithDefaultTimeout()
		newBBinspect := inspect.InspectImageJSON()
		Expect(oldBBinspect[0].Digest).To(Not(Equal(newBBinspect[0].Digest)))
	})

	It("podman play kube with latest image should always pull", func() {
		oldBB := "quay.io/libpod/busybox:1.30.1"
		pull := podmanTest.Podman([]string{"pull", oldBB})
		pull.WaitWithDefaultTimeout()

		tag := podmanTest.Podman([]string{"tag", oldBB, BB})
		tag.WaitWithDefaultTimeout()
		Expect(tag.ExitCode()).To(BeZero())

		rmi := podmanTest.Podman([]string{"rmi", oldBB})
		rmi.WaitWithDefaultTimeout()
		Expect(rmi.ExitCode()).To(BeZero())

		inspect := podmanTest.Podman([]string{"inspect", BB})
		inspect.WaitWithDefaultTimeout()
		oldBBinspect := inspect.InspectImageJSON()

		ctr := getCtr(withImage(BB))
		err := generateKubeYaml("pod", getPod(withCtr(ctr)), kubeYaml)
		Expect(err).To(BeNil())

		kube := podmanTest.Podman([]string{"play", "kube", kubeYaml})
		kube.WaitWithDefaultTimeout()
		Expect(kube.ExitCode()).To(Equal(0))

		inspect = podmanTest.Podman([]string{"inspect", BB})
		inspect.WaitWithDefaultTimeout()
		newBBinspect := inspect.InspectImageJSON()
		Expect(oldBBinspect[0].Digest).To(Not(Equal(newBBinspect[0].Digest)))
	})

	It("podman play kube with image data", func() {
		testyaml := `
apiVersion: v1
kind: Pod
metadata:
  name: demo_pod
spec:
  containers:
  - image: demo
    name: demo_kube
`
		pull := podmanTest.Podman([]string{"create", "--workdir", "/etc", "--name", "newBB", "--label", "key1=value1", "alpine"})

		pull.WaitWithDefaultTimeout()
		Expect(pull.ExitCode()).To(BeZero())

		c := podmanTest.Podman([]string{"commit", "-c", "STOPSIGNAL=51", "newBB", "demo"})
		c.WaitWithDefaultTimeout()
		Expect(c.ExitCode()).To(Equal(0))

		conffile := filepath.Join(podmanTest.TempDir, "kube.yaml")
		tempdir, err = CreateTempDirInTempDir()
		Expect(err).To(BeNil())

		err := ioutil.WriteFile(conffile, []byte(testyaml), 0755)
		Expect(err).To(BeNil())

		kube := podmanTest.Podman([]string{"play", "kube", conffile})
		kube.WaitWithDefaultTimeout()
		Expect(kube.ExitCode()).To(Equal(0))

		inspect := podmanTest.Podman([]string{"inspect", "demo_pod-demo_kube"})
		inspect.WaitWithDefaultTimeout()
		Expect(inspect.ExitCode()).To(Equal(0))

		ctr := inspect.InspectContainerToJSON()
		Expect(ctr[0].Config.WorkingDir).To(ContainSubstring("/etc"))
		Expect(ctr[0].Config.Labels["key1"]).To(ContainSubstring("value1"))
		Expect(ctr[0].Config.Labels["key1"]).To(ContainSubstring("value1"))
		Expect(ctr[0].Config.StopSignal).To(Equal(uint(51)))
	})

	// Deployment related tests
	It("podman play kube deployment 1 replica test correct command", func() {
		deployment := getDeployment()
		err := generateKubeYaml("deployment", deployment, kubeYaml)
		Expect(err).To(BeNil())

		kube := podmanTest.Podman([]string{"play", "kube", kubeYaml})
		kube.WaitWithDefaultTimeout()
		Expect(kube.ExitCode()).To(Equal(0))

		podNames := getPodNamesInDeployment(deployment)
		inspect := podmanTest.Podman([]string{"inspect", getCtrNameInPod(&podNames[0]), "--format", "'{{ .Config.Entrypoint }}'"})
		inspect.WaitWithDefaultTimeout()
		Expect(inspect.ExitCode()).To(Equal(0))
		// yaml's command should override the image's Entrypoint
		Expect(inspect.OutputToString()).To(ContainSubstring(strings.Join(defaultCtrCmd, " ")))
	})

	It("podman play kube deployment more than 1 replica test correct command", func() {
		var i, numReplicas int32
		numReplicas = 5
		deployment := getDeployment(withReplicas(numReplicas))
		err := generateKubeYaml("deployment", deployment, kubeYaml)
		Expect(err).To(BeNil())

		kube := podmanTest.Podman([]string{"play", "kube", kubeYaml})
		kube.WaitWithDefaultTimeout()
		Expect(kube.ExitCode()).To(Equal(0))

		podNames := getPodNamesInDeployment(deployment)
		for i = 0; i < numReplicas; i++ {
			inspect := podmanTest.Podman([]string{"inspect", getCtrNameInPod(&podNames[i]), "--format", "'{{ .Config.Entrypoint }}'"})
			inspect.WaitWithDefaultTimeout()
			Expect(inspect.ExitCode()).To(Equal(0))
			Expect(inspect.OutputToString()).To(ContainSubstring(strings.Join(defaultCtrCmd, " ")))
		}
	})

	It("podman play kube test with network portbindings", func() {
		ip := "127.0.0.100"
		port := "5000"
		ctr := getCtr(withHostIP(ip, port), withImage(BB))

		pod := getPod(withCtr(ctr))
		err := generateKubeYaml("pod", pod, kubeYaml)
		Expect(err).To(BeNil())

		kube := podmanTest.Podman([]string{"play", "kube", kubeYaml})
		kube.WaitWithDefaultTimeout()
		Expect(kube.ExitCode()).To(Equal(0))

		inspect := podmanTest.Podman([]string{"port", getCtrNameInPod(pod)})
		inspect.WaitWithDefaultTimeout()
		Expect(inspect.ExitCode()).To(Equal(0))
		Expect(inspect.OutputToString()).To(Equal("5000/tcp -> 127.0.0.100:5000"))
	})

	It("podman play kube test with nonexistent empty HostPath type volume", func() {
		hostPathLocation := filepath.Join(tempdir, "file")

		pod := getPod(withVolume(getHostPathVolume(`""`, hostPathLocation)))
		err := generateKubeYaml("pod", pod, kubeYaml)
		Expect(err).To(BeNil())

		kube := podmanTest.Podman([]string{"play", "kube", kubeYaml})
		kube.WaitWithDefaultTimeout()
		Expect(kube.ExitCode()).NotTo(Equal(0))
		Expect(kube.ErrorToString()).To(ContainSubstring(defaultVolName))
	})

	It("podman play kube test with empty HostPath type volume", func() {
		hostPathLocation := filepath.Join(tempdir, "file")
		f, err := os.Create(hostPathLocation)
		Expect(err).To(BeNil())
		f.Close()

		pod := getPod(withVolume(getHostPathVolume(`""`, hostPathLocation)))
		err = generateKubeYaml("pod", pod, kubeYaml)
		Expect(err).To(BeNil())

		kube := podmanTest.Podman([]string{"play", "kube", kubeYaml})
		kube.WaitWithDefaultTimeout()
		Expect(kube.ExitCode()).To(Equal(0))
	})

	It("podman play kube test with nonexistent File HostPath type volume", func() {
		hostPathLocation := filepath.Join(tempdir, "file")

		pod := getPod(withVolume(getHostPathVolume("File", hostPathLocation)))
		err := generateKubeYaml("pod", pod, kubeYaml)
		Expect(err).To(BeNil())

		kube := podmanTest.Podman([]string{"play", "kube", kubeYaml})
		kube.WaitWithDefaultTimeout()
		Expect(kube.ExitCode()).NotTo(Equal(0))
	})

	It("podman play kube test with File HostPath type volume", func() {
		hostPathLocation := filepath.Join(tempdir, "file")
		f, err := os.Create(hostPathLocation)
		Expect(err).To(BeNil())
		f.Close()

		pod := getPod(withVolume(getHostPathVolume("File", hostPathLocation)))
		err = generateKubeYaml("pod", pod, kubeYaml)
		Expect(err).To(BeNil())

		kube := podmanTest.Podman([]string{"play", "kube", kubeYaml})
		kube.WaitWithDefaultTimeout()
		Expect(kube.ExitCode()).To(Equal(0))
	})

	It("podman play kube test with FileOrCreate HostPath type volume", func() {
		hostPathLocation := filepath.Join(tempdir, "file")

		pod := getPod(withVolume(getHostPathVolume("FileOrCreate", hostPathLocation)))
		err := generateKubeYaml("pod", pod, kubeYaml)
		Expect(err).To(BeNil())

		kube := podmanTest.Podman([]string{"play", "kube", kubeYaml})
		kube.WaitWithDefaultTimeout()
		Expect(kube.ExitCode()).To(Equal(0))

		// the file should have been created
		_, err = os.Stat(hostPathLocation)
		Expect(err).To(BeNil())
	})

	It("podman play kube test with DirectoryOrCreate HostPath type volume", func() {
		hostPathLocation := filepath.Join(tempdir, "file")

		pod := getPod(withVolume(getHostPathVolume("DirectoryOrCreate", hostPathLocation)))
		err := generateKubeYaml("pod", pod, kubeYaml)
		Expect(err).To(BeNil())

		kube := podmanTest.Podman([]string{"play", "kube", kubeYaml})
		kube.WaitWithDefaultTimeout()
		Expect(kube.ExitCode()).To(Equal(0))

		// the file should have been created
		st, err := os.Stat(hostPathLocation)
		Expect(err).To(BeNil())
		Expect(st.Mode().IsDir()).To(Equal(true))
	})

	It("podman play kube test with Socket HostPath type volume should fail if not socket", func() {
		hostPathLocation := filepath.Join(tempdir, "file")
		f, err := os.Create(hostPathLocation)
		Expect(err).To(BeNil())
		f.Close()

		pod := getPod(withVolume(getHostPathVolume("Socket", hostPathLocation)))
		err = generateKubeYaml("pod", pod, kubeYaml)
		Expect(err).To(BeNil())

		kube := podmanTest.Podman([]string{"play", "kube", kubeYaml})
		kube.WaitWithDefaultTimeout()
		Expect(kube.ExitCode()).NotTo(Equal(0))
	})

	It("podman play kube test with read only HostPath volume", func() {
		hostPathLocation := filepath.Join(tempdir, "file")
		f, err := os.Create(hostPathLocation)
		Expect(err).To(BeNil())
		f.Close()

		ctr := getCtr(withVolumeMount(hostPathLocation, true), withImage(BB))
		pod := getPod(withVolume(getHostPathVolume("File", hostPathLocation)), withCtr(ctr))
		err = generateKubeYaml("pod", pod, kubeYaml)
		Expect(err).To(BeNil())

		kube := podmanTest.Podman([]string{"play", "kube", kubeYaml})
		kube.WaitWithDefaultTimeout()
		Expect(kube.ExitCode()).To(Equal(0))

		inspect := podmanTest.Podman([]string{"inspect", getCtrNameInPod(pod), "--format", "'{{.HostConfig.Binds}}'"})
		inspect.WaitWithDefaultTimeout()
		Expect(inspect.ExitCode()).To(Equal(0))

		correct := fmt.Sprintf("%s:%s:%s", hostPathLocation, hostPathLocation, "ro")
		Expect(inspect.OutputToString()).To(ContainSubstring(correct))
	})

	It("podman play kube test with PersistentVolumeClaim volume", func() {
		volumeName := "namedVolume"

		ctr := getCtr(withVolumeMount("/test", false), withImage(BB))
		pod := getPod(withVolume(getPersistentVolumeClaimVolume(volumeName)), withCtr(ctr))
		err = generateKubeYaml("pod", pod, kubeYaml)
		Expect(err).To(BeNil())

		kube := podmanTest.Podman([]string{"play", "kube", kubeYaml})
		kube.WaitWithDefaultTimeout()
		Expect(kube.ExitCode()).To(Equal(0))

		inspect := podmanTest.Podman([]string{"inspect", getCtrNameInPod(pod), "--format", "{{ (index .Mounts 0).Type }}:{{ (index .Mounts 0).Name }}"})
		inspect.WaitWithDefaultTimeout()
		Expect(inspect.ExitCode()).To(Equal(0))

		correct := fmt.Sprintf("volume:%s", volumeName)
		Expect(inspect.OutputToString()).To(Equal(correct))
	})

	It("podman play kube applies labels to pods", func() {
		var numReplicas int32 = 5
		expectedLabelKey := "key1"
		expectedLabelValue := "value1"
		deployment := getDeployment(
			withReplicas(numReplicas),
			withPod(getPod(withLabel(expectedLabelKey, expectedLabelValue))),
		)
		err := generateKubeYaml("deployment", deployment, kubeYaml)
		Expect(err).To(BeNil())

		kube := podmanTest.Podman([]string{"play", "kube", kubeYaml})
		kube.WaitWithDefaultTimeout()
		Expect(kube.ExitCode()).To(Equal(0))

		correctLabels := expectedLabelKey + ":" + expectedLabelValue
		for _, pod := range getPodNamesInDeployment(deployment) {
			inspect := podmanTest.Podman([]string{"pod", "inspect", pod.Name, "--format", "'{{ .Labels }}'"})
			inspect.WaitWithDefaultTimeout()
			Expect(inspect.ExitCode()).To(Equal(0))
			Expect(inspect.OutputToString()).To(ContainSubstring(correctLabels))
		}
	})

	It("podman play kube allows setting resource limits", func() {
		SkipIfContainerized("Resource limits require a running systemd")
		SkipIfRootlessCgroupsV1("Limits require root or cgroups v2")
		SkipIfUnprivilegedCPULimits()
		podmanTest.CgroupManager = "systemd"

		var (
			numReplicas           int32  = 3
			expectedCpuRequest    string = "100m"
			expectedCpuLimit      string = "200m"
			expectedMemoryRequest string = "10000000"
			expectedMemoryLimit   string = "20000000"
		)

		expectedCpuQuota := milliCPUToQuota(expectedCpuLimit)

		deployment := getDeployment(
			withReplicas(numReplicas),
			withPod(getPod(withCtr(getCtr(
				withCpuRequest(expectedCpuRequest),
				withCpuLimit(expectedCpuLimit),
				withMemoryRequest(expectedMemoryRequest),
				withMemoryLimit(expectedMemoryLimit),
			)))))
		err := generateKubeYaml("deployment", deployment, kubeYaml)
		Expect(err).To(BeNil())

		kube := podmanTest.Podman([]string{"play", "kube", kubeYaml})
		kube.WaitWithDefaultTimeout()
		Expect(kube.ExitCode()).To(Equal(0))

		for _, pod := range getPodNamesInDeployment(deployment) {
			inspect := podmanTest.Podman([]string{"inspect", getCtrNameInPod(&pod), "--format", `
CpuPeriod: {{ .HostConfig.CpuPeriod }}
CpuQuota: {{ .HostConfig.CpuQuota }}
Memory: {{ .HostConfig.Memory }}
MemoryReservation: {{ .HostConfig.MemoryReservation }}`})
			inspect.WaitWithDefaultTimeout()
			Expect(inspect.ExitCode()).To(Equal(0))
			Expect(inspect.OutputToString()).To(ContainSubstring(fmt.Sprintf("%s: %d", "CpuQuota", expectedCpuQuota)))
			Expect(inspect.OutputToString()).To(ContainSubstring("MemoryReservation: " + expectedMemoryRequest))
			Expect(inspect.OutputToString()).To(ContainSubstring("Memory: " + expectedMemoryLimit))
		}
	})

	It("podman play kube reports invalid image name", func() {
		invalidImageName := "./myimage"

		pod := getPod(
			withCtr(
				getCtr(
					withImage(invalidImageName),
				),
			),
		)
		err := generateKubeYaml("pod", pod, kubeYaml)
		Expect(err).To(BeNil())

		kube := podmanTest.Podman([]string{"play", "kube", kubeYaml})
		kube.WaitWithDefaultTimeout()
		Expect(kube.ExitCode()).To(Equal(125))
		Expect(kube.ErrorToString()).To(ContainSubstring(invalidImageName))
	})

	It("podman play kube applies log driver to containers", func() {
		Skip("need to verify images have correct packages for journald")
		pod := getPod()
		err := generateKubeYaml("pod", pod, kubeYaml)
		Expect(err).To(BeNil())

		kube := podmanTest.Podman([]string{"play", "kube", "--log-driver", "journald", kubeYaml})
		kube.WaitWithDefaultTimeout()
		Expect(kube.ExitCode()).To(Equal(0))

		inspect := podmanTest.Podman([]string{"inspect", getCtrNameInPod(pod), "--format", "'{{ .HostConfig.LogConfig.Type }}'"})
		inspect.WaitWithDefaultTimeout()
		Expect(inspect.ExitCode()).To(Equal(0))
		Expect(inspect.OutputToString()).To(ContainSubstring("journald"))
	})

	It("podman play kube test only creating the containers", func() {
		pod := getPod()
		err := generateKubeYaml("pod", pod, kubeYaml)
		Expect(err).To(BeNil())

		kube := podmanTest.Podman([]string{"play", "kube", "--start=false", kubeYaml})
		kube.WaitWithDefaultTimeout()
		Expect(kube.ExitCode()).To(Equal(0))

		inspect := podmanTest.Podman([]string{"inspect", getCtrNameInPod(pod), "--format", "{{ .State.Running }}"})
		inspect.WaitWithDefaultTimeout()
		Expect(inspect.ExitCode()).To(Equal(0))
		Expect(inspect.OutputToString()).To(Equal("false"))
	})

	It("podman play kube test with HostNetwork", func() {
		if !strings.Contains(podmanTest.OCIRuntime, "crun") {
			Skip("Test only works on crun")
		}

		pod := getPod(withHostNetwork())
		err := generateKubeYaml("pod", pod, kubeYaml)
		Expect(err).To(BeNil())

		kube := podmanTest.Podman([]string{"play", "kube", kubeYaml})
		kube.WaitWithDefaultTimeout()
		Expect(kube.ExitCode()).To(Equal(0))

		inspect := podmanTest.Podman([]string{"inspect", pod.Name, "--format", "{{ .InfraConfig.HostNetwork }}"})
		inspect.WaitWithDefaultTimeout()
		Expect(inspect.ExitCode()).To(Equal(0))
		Expect(inspect.OutputToString()).To(Equal("true"))
	})

	It("podman play kube persistentVolumeClaim", func() {
		volName := "myvol"
		volDevice := "tmpfs"
		volType := "tmpfs"
		volOpts := "nodev,noexec"

		pvc := getPVC(withPVCName(volName),
			withPVCAnnotations(util.VolumeDeviceAnnotation, volDevice),
			withPVCAnnotations(util.VolumeTypeAnnotation, volType),
			withPVCAnnotations(util.VolumeMountOptsAnnotation, volOpts))
		err = generateKubeYaml("persistentVolumeClaim", pvc, kubeYaml)
		Expect(err).To(BeNil())

		kube := podmanTest.Podman([]string{"play", "kube", kubeYaml})
		kube.WaitWithDefaultTimeout()
		Expect(kube.ExitCode()).To(Equal(0))

		inspect := podmanTest.Podman([]string{"inspect", volName, "--format", `
Name: {{ .Name }}
Device: {{ .Options.device }}
Type: {{ .Options.type }}
o: {{ .Options.o }}`})
		inspect.WaitWithDefaultTimeout()
		Expect(inspect.ExitCode()).To(Equal(0))
		Expect(inspect.OutputToString()).To(ContainSubstring("Name: " + volName))
		Expect(inspect.OutputToString()).To(ContainSubstring("Device: " + volDevice))
		Expect(inspect.OutputToString()).To(ContainSubstring("Type: " + volType))
		Expect(inspect.OutputToString()).To(ContainSubstring("o: " + volOpts))
	})

	// Multi doc related tests
	It("podman play kube multi doc yaml with persistentVolumeClaim, service and deployment", func() {
		yamlDocs := []string{}

		serviceTemplate := `apiVersion: v1
kind: Service
metadata:
  name: %s
spec:
  ports:
  - port: 80
    protocol: TCP
    targetPort: 9376
  selector:
    app: %s
`
		// generate persistentVolumeClaim
		volName := "multiFoo"
		pvc := getPVC(withPVCName(volName))

		// generate deployment
		deploymentName := "multiFoo"
		podName := "multiFoo"
		ctrName := "ctr-01"
		ctr := getCtr(withVolumeMount("/test", false))
		ctr.Name = ctrName
		pod := getPod(withPodName(podName), withVolume(getPersistentVolumeClaimVolume(volName)), withCtr(ctr))
		deployment := getDeployment(withPod(pod))
		deployment.Name = deploymentName

		// add pvc
		k, err := getKubeYaml("persistentVolumeClaim", pvc)
		Expect(err).To(BeNil())
		yamlDocs = append(yamlDocs, k)

		// add service
		yamlDocs = append(yamlDocs, fmt.Sprintf(serviceTemplate, deploymentName, deploymentName))

		// add deployment
		k, err = getKubeYaml("deployment", deployment)
		Expect(err).To(BeNil())
		yamlDocs = append(yamlDocs, k)

		// generate multi doc yaml
		err = generateMultiDocKubeYaml(yamlDocs, kubeYaml)
		Expect(err).To(BeNil())

		kube := podmanTest.Podman([]string{"play", "kube", kubeYaml})
		kube.WaitWithDefaultTimeout()
		Expect(kube.ExitCode()).To(Equal(0))

		inspectVolume := podmanTest.Podman([]string{"inspect", volName, "--format", "'{{ .Name }}'"})
		inspectVolume.WaitWithDefaultTimeout()
		Expect(inspectVolume.ExitCode()).To(Equal(0))
		Expect(inspectVolume.OutputToString()).To(ContainSubstring(volName))

		inspectPod := podmanTest.Podman([]string{"inspect", podName + "-pod-0", "--format", "'{{ .State }}'"})
		inspectPod.WaitWithDefaultTimeout()
		Expect(inspectPod.ExitCode()).To(Equal(0))
		Expect(inspectPod.OutputToString()).To(ContainSubstring(`Running`))

		inspectMounts := podmanTest.Podman([]string{"inspect", podName + "-pod-0-" + ctrName, "--format", "{{ (index .Mounts 0).Type }}:{{ (index .Mounts 0).Name }}"})
		inspectMounts.WaitWithDefaultTimeout()
		Expect(inspectMounts.ExitCode()).To(Equal(0))

		correct := fmt.Sprintf("volume:%s", volName)
		Expect(inspectMounts.OutputToString()).To(Equal(correct))
	})

	It("podman play kube multi doc yaml with multiple services, pods and deployments", func() {
		yamlDocs := []string{}
		podNames := []string{}

		serviceTemplate := `apiVersion: v1
kind: Service
metadata:
  name: %s
spec:
  ports:
  - port: 80
    protocol: TCP
    targetPort: 9376
  selector:
    app: %s
`
		// generate services, pods and deployments
		for i := 0; i < 2; i++ {
			podName := fmt.Sprintf("testPod%d", i)
			deploymentName := fmt.Sprintf("testDeploy%d", i)
			deploymentPodName := fmt.Sprintf("%s-pod-0", deploymentName)

			podNames = append(podNames, podName)
			podNames = append(podNames, deploymentPodName)

			pod := getPod(withPodName(podName))
			podDeployment := getPod(withPodName(deploymentName))
			deployment := getDeployment(withPod(podDeployment))
			deployment.Name = deploymentName

			// add services
			yamlDocs = append([]string{
				fmt.Sprintf(serviceTemplate, podName, podName),
				fmt.Sprintf(serviceTemplate, deploymentPodName, deploymentPodName)}, yamlDocs...)

			// add pods
			k, err := getKubeYaml("pod", pod)
			Expect(err).To(BeNil())
			yamlDocs = append(yamlDocs, k)

			// add deployments
			k, err = getKubeYaml("deployment", deployment)
			Expect(err).To(BeNil())
			yamlDocs = append(yamlDocs, k)
		}

		// generate multi doc yaml
		err = generateMultiDocKubeYaml(yamlDocs, kubeYaml)
		Expect(err).To(BeNil())

		kube := podmanTest.Podman([]string{"play", "kube", kubeYaml})
		kube.WaitWithDefaultTimeout()
		Expect(kube.ExitCode()).To(Equal(0))

		for _, n := range podNames {
			inspect := podmanTest.Podman([]string{"inspect", n, "--format", "'{{ .State }}'"})
			inspect.WaitWithDefaultTimeout()
			Expect(inspect.ExitCode()).To(Equal(0))
			Expect(inspect.OutputToString()).To(ContainSubstring(`Running`))
		}
	})

	It("podman play kube invalid multi doc yaml", func() {
		yamlDocs := []string{}

		serviceTemplate := `apiVersion: v1
kind: Service
metadata:
  name: %s
spec:
  ports:
  - port: 80
    protocol: TCP
    targetPort: 9376
  selector:
	app: %s
---
invalid kube kind
`
		// add invalid multi doc yaml
		yamlDocs = append(yamlDocs, fmt.Sprintf(serviceTemplate, "foo", "foo"))

		// add pod
		pod := getPod()
		k, err := getKubeYaml("pod", pod)
		Expect(err).To(BeNil())
		yamlDocs = append(yamlDocs, k)

		// generate multi doc yaml
		err = generateMultiDocKubeYaml(yamlDocs, kubeYaml)
		Expect(err).To(BeNil())

		kube := podmanTest.Podman([]string{"play", "kube", kubeYaml})
		kube.WaitWithDefaultTimeout()
		Expect(kube.ExitCode()).To(Not(Equal(0)))
	})
})
