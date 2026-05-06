package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"gopkg.in/yaml.v3"
)

type heartbeatRequest struct {
	AgentCode          string   `json:"agent_code"`
	Token              string   `json:"token"`
	Hostname           string   `json:"hostname"`
	HostIP             string   `json:"host_ip"`
	AgentVersion       string   `json:"agent_version"`
	OS                 string   `json:"os"`
	Arch               string   `json:"arch"`
	WorkDir            string   `json:"work_dir"`
	Tags               []string `json:"tags"`
	CurrentTaskID      string   `json:"current_task_id,omitempty"`
	CurrentTaskName    string   `json:"current_task_name,omitempty"`
	CurrentTaskType    string   `json:"current_task_type,omitempty"`
	CurrentTaskStarted string   `json:"current_task_started_at,omitempty"`
	LastTaskStatus     string   `json:"last_task_status,omitempty"`
	LastTaskSummary    string   `json:"last_task_summary,omitempty"`
	LastTaskFinishedAt string   `json:"last_task_finished_at,omitempty"`
}

type fileConfig struct {
	Server struct {
		BaseURL string `yaml:"base_url"`
	} `yaml:"server"`
	Agent struct {
		Code              string   `yaml:"code"`
		Token             string   `yaml:"token"`
		RegistrationToken string   `yaml:"registration_token"`
		Name              string   `yaml:"name"`
		EnvironmentCode   string   `yaml:"environment_code"`
		WorkDir           string   `yaml:"work_dir"`
		HeartbeatInterval string   `yaml:"heartbeat_interval"`
		PollInterval      string   `yaml:"poll_interval"`
		Version           string   `yaml:"version"`
		Tags              []string `yaml:"tags"`
	} `yaml:"agent"`
}

type runtimeConfig struct {
	BaseURL           string
	AgentCode         string
	Token             string
	RegistrationToken string
	AgentName         string
	EnvironmentCode   string
	WorkDir           string
	Version           string
	Tags              []string
	HeartbeatInterval time.Duration
	PollInterval      time.Duration
}

type registerRequest struct {
	RegistrationToken string   `json:"registration_token"`
	MachineID         string   `json:"machine_id"`
	Name              string   `json:"name"`
	EnvironmentCode   string   `json:"environment_code"`
	Hostname          string   `json:"hostname"`
	HostIP            string   `json:"host_ip"`
	AgentVersion      string   `json:"agent_version"`
	OS                string   `json:"os"`
	Arch              string   `json:"arch"`
	WorkDir           string   `json:"work_dir"`
	Tags              []string `json:"tags"`
}

type registerResponse struct {
	Data *registerOutput `json:"data"`
}

type registerOutput struct {
	AgentID    string `json:"agent_id"`
	AgentCode  string `json:"agent_code"`
	Token      string `json:"token"`
	Name       string `json:"name"`
	WorkDir    string `json:"work_dir"`
	Registered bool   `json:"registered"`
}

type agentLocalState struct {
	MachineID string `json:"machine_id"`
	AgentCode string `json:"agent_code"`
	Token     string `json:"token"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

type taskOutput struct {
	ID         string            `json:"id"`
	AgentID    string            `json:"agent_id"`
	AgentCode  string            `json:"agent_code"`
	Name       string            `json:"name"`
	TaskType   string            `json:"task_type"`
	ShellType  string            `json:"shell_type"`
	WorkDir    string            `json:"work_dir"`
	ScriptPath string            `json:"script_path"`
	ScriptText string            `json:"script_text"`
	Variables  map[string]string `json:"variables"`
	TimeoutSec int               `json:"timeout_sec"`
	Status     string            `json:"status"`
}

type taskResponse struct {
	Data *taskOutput `json:"data"`
}

type taskPollRequest struct {
	AgentCode string `json:"agent_code"`
	Token     string `json:"token"`
}

type taskFinishRequest struct {
	AgentCode     string `json:"agent_code"`
	Token         string `json:"token"`
	Status        string `json:"status"`
	ExitCode      int    `json:"exit_code"`
	StdoutText    string `json:"stdout_text"`
	StderrText    string `json:"stderr_text"`
	FailureReason string `json:"failure_reason"`
}

type runtimeState struct {
	mu                 sync.RWMutex
	currentTaskID      string
	currentTaskName    string
	currentTaskType    string
	currentTaskStarted string
	lastTaskStatus     string
	lastTaskSummary    string
	lastTaskFinishedAt string
}

const maxCapturedOutputBytes = 64 * 1024

// snapshot 封装当前模块的业务处理逻辑。
func (s *runtimeState) snapshot() heartbeatRequest {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return heartbeatRequest{
		CurrentTaskID:      s.currentTaskID,
		CurrentTaskName:    s.currentTaskName,
		CurrentTaskType:    s.currentTaskType,
		CurrentTaskStarted: s.currentTaskStarted,
		LastTaskStatus:     s.lastTaskStatus,
		LastTaskSummary:    s.lastTaskSummary,
		LastTaskFinishedAt: s.lastTaskFinishedAt,
	}
}

// markRunning 封装当前模块的业务处理逻辑。
func (s *runtimeState) markRunning(task *taskOutput, now time.Time) {
	if task == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.currentTaskID = task.ID
	s.currentTaskName = task.Name
	s.currentTaskType = task.TaskType
	s.currentTaskStarted = now.UTC().Format(time.RFC3339)
	s.lastTaskStatus = "running"
	s.lastTaskSummary = "任务执行中"
	s.lastTaskFinishedAt = ""
}

// markFinished 封装当前模块的业务处理逻辑。
func (s *runtimeState) markFinished(status, summary string, finishedAt time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.currentTaskID = ""
	s.currentTaskName = ""
	s.currentTaskType = ""
	s.currentTaskStarted = ""
	s.lastTaskStatus = status
	s.lastTaskSummary = trimText(summary, 500)
	s.lastTaskFinishedAt = finishedAt.UTC().Format(time.RFC3339)
}

// main 启动当前进程并完成依赖初始化。
func main() {
	var (
		configPath        = flag.String("config", strings.TrimSpace(os.Getenv("GOS_AGENT_CONFIG")), "agent config file path")
		baseURL           = flag.String("base-url", envOrDefault("GOS_AGENT_BASE_URL", ""), "GOS server base url")
		agentCode         = flag.String("agent-code", envOrDefault("GOS_AGENT_CODE", ""), "agent code")
		token             = flag.String("token", envOrDefault("GOS_AGENT_TOKEN", ""), "agent token")
		registrationToken = flag.String("registration-token", envOrDefault("GOS_AGENT_REGISTRATION_TOKEN", ""), "agent bootstrap registration token")
		agentName         = flag.String("name", envOrDefault("GOS_AGENT_NAME", ""), "agent display name")
		environmentCode   = flag.String("environment-code", envOrDefault("GOS_AGENT_ENVIRONMENT_CODE", ""), "agent environment code")
		workDir           = flag.String("work-dir", envOrDefault("GOS_AGENT_WORK_DIR", currentDir()), "agent work dir")
		version           = flag.String("version", envOrDefault("GOS_AGENT_VERSION", "dev"), "agent version")
		tagsRaw           = flag.String("tags", envOrDefault("GOS_AGENT_TAGS", ""), "comma separated tags")
		hbInterval        = flag.Duration("heartbeat-interval", envDurationOrDefault("GOS_AGENT_HEARTBEAT_INTERVAL", 15*time.Second), "heartbeat interval")
		pollIntvl         = flag.Duration("poll-interval", envDurationOrDefault("GOS_AGENT_POLL_INTERVAL", 5*time.Second), "task poll interval")
	)
	flag.Parse()

	cfg, err := loadRuntimeConfig(*configPath, runtimeConfig{
		BaseURL:           strings.TrimSpace(*baseURL),
		AgentCode:         strings.TrimSpace(*agentCode),
		Token:             strings.TrimSpace(*token),
		RegistrationToken: strings.TrimSpace(*registrationToken),
		AgentName:         strings.TrimSpace(*agentName),
		EnvironmentCode:   strings.TrimSpace(*environmentCode),
		WorkDir:           strings.TrimSpace(*workDir),
		Version:           strings.TrimSpace(*version),
		Tags:              parseTags(*tagsRaw),
		HeartbeatInterval: *hbInterval,
		PollInterval:      *pollIntvl,
	})
	if err != nil {
		log.Fatal(err)
	}

	if err := os.MkdirAll(cfg.WorkDir, 0o755); err != nil {
		log.Fatalf("prepare work dir failed: %v", err)
	}

	hostname, _ := os.Hostname()
	hostIP := discoverHostIP()
	cfg.AgentName = resolveAgentName(cfg.AgentName, hostname, hostIP)
	client := &http.Client{Timeout: 15 * time.Second}
	statePath := agentStateFilePath(cfg.WorkDir)
	localState, err := loadAgentLocalState(statePath)
	if err != nil {
		log.Printf("load local state failed: %v", err)
	}
	cfg, err = bootstrapRuntimeCredentials(client, cfg, hostname, hostIP, statePath, localState)
	if err != nil {
		log.Fatal(err)
	}
	hbTicker := time.NewTicker(cfg.HeartbeatInterval)
	pollTicker := time.NewTicker(cfg.PollInterval)
	defer hbTicker.Stop()
	defer pollTicker.Stop()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	state := &runtimeState{}
	var runningTask sync.Mutex
	taskExecuting := false

	log.Printf(
		"gos-agent starting: name=%s code=%s base_url=%s work_dir=%s heartbeat=%s poll=%s",
		cfg.AgentName, cfg.AgentCode, cfg.BaseURL, cfg.WorkDir, cfg.HeartbeatInterval.String(), cfg.PollInterval.String(),
	)
	if err := sendHeartbeat(client, cfg, hostname, hostIP, state); err != nil {
		log.Printf("initial heartbeat failed: %v", err)
	} else {
		log.Printf("initial heartbeat sent")
	}

	for {
		select {
		case <-hbTicker.C:
			if err := sendHeartbeat(client, cfg, hostname, hostIP, state); err != nil {
				log.Printf("heartbeat failed: %v", err)
				continue
			}
			log.Printf("heartbeat sent")
		case <-pollTicker.C:
			runningTask.Lock()
			executing := taskExecuting
			runningTask.Unlock()
			if executing {
				continue
			}
			task, err := pollTask(client, cfg)
			if err != nil {
				log.Printf("poll task failed: %v", err)
				continue
			}
			if task == nil {
				continue
			}
			runningTask.Lock()
			taskExecuting = true
			runningTask.Unlock()
			go func(task *taskOutput) {
				defer func() {
					runningTask.Lock()
					taskExecuting = false
					runningTask.Unlock()
				}()
				executeTask(client, cfg, task, hostname, hostIP, state)
			}(task)
		case sig := <-stop:
			log.Printf("received signal %s, exiting", sig)
			return
		}
	}
}

// loadRuntimeConfig 封装当前模块的业务处理逻辑。
func loadRuntimeConfig(configPath string, fallback runtimeConfig) (runtimeConfig, error) {
	cfg := fallback
	if strings.TrimSpace(configPath) != "" {
		fileCfg, err := readConfigFile(configPath)
		if err != nil {
			return runtimeConfig{}, err
		}
		if strings.TrimSpace(fileCfg.Server.BaseURL) != "" {
			cfg.BaseURL = strings.TrimSpace(fileCfg.Server.BaseURL)
		}
		if strings.TrimSpace(fileCfg.Agent.Code) != "" {
			cfg.AgentCode = strings.TrimSpace(fileCfg.Agent.Code)
		}
		if strings.TrimSpace(fileCfg.Agent.Token) != "" {
			cfg.Token = strings.TrimSpace(fileCfg.Agent.Token)
		}
		if strings.TrimSpace(fileCfg.Agent.RegistrationToken) != "" {
			cfg.RegistrationToken = strings.TrimSpace(fileCfg.Agent.RegistrationToken)
		}
		if strings.TrimSpace(fileCfg.Agent.Name) != "" {
			cfg.AgentName = strings.TrimSpace(fileCfg.Agent.Name)
		}
		if strings.TrimSpace(fileCfg.Agent.EnvironmentCode) != "" {
			cfg.EnvironmentCode = strings.TrimSpace(fileCfg.Agent.EnvironmentCode)
		}
		if strings.TrimSpace(fileCfg.Agent.WorkDir) != "" {
			cfg.WorkDir = strings.TrimSpace(fileCfg.Agent.WorkDir)
		}
		if strings.TrimSpace(fileCfg.Agent.Version) != "" {
			cfg.Version = strings.TrimSpace(fileCfg.Agent.Version)
		}
		if len(fileCfg.Agent.Tags) > 0 {
			cfg.Tags = normalizeTags(fileCfg.Agent.Tags)
		}
		if strings.TrimSpace(fileCfg.Agent.HeartbeatInterval) != "" {
			parsed, err := time.ParseDuration(strings.TrimSpace(fileCfg.Agent.HeartbeatInterval))
			if err != nil {
				return runtimeConfig{}, fmt.Errorf("invalid heartbeat_interval in config: %w", err)
			}
			cfg.HeartbeatInterval = parsed
		}
		if strings.TrimSpace(fileCfg.Agent.PollInterval) != "" {
			parsed, err := time.ParseDuration(strings.TrimSpace(fileCfg.Agent.PollInterval))
			if err != nil {
				return runtimeConfig{}, fmt.Errorf("invalid poll_interval in config: %w", err)
			}
			cfg.PollInterval = parsed
		}
	}
	cfg.BaseURL = strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	cfg.AgentCode = strings.TrimSpace(cfg.AgentCode)
	cfg.Token = strings.TrimSpace(cfg.Token)
	cfg.RegistrationToken = strings.TrimSpace(cfg.RegistrationToken)
	cfg.AgentName = strings.TrimSpace(cfg.AgentName)
	cfg.EnvironmentCode = strings.TrimSpace(cfg.EnvironmentCode)
	cfg.WorkDir = strings.TrimSpace(cfg.WorkDir)
	cfg.Version = strings.TrimSpace(cfg.Version)
	cfg.Tags = normalizeTags(cfg.Tags)
	if cfg.BaseURL == "" {
		cfg.BaseURL = "http://127.0.0.1:8081"
	}
	if cfg.WorkDir == "" {
		cfg.WorkDir = currentDir()
	}
	if cfg.Version == "" {
		cfg.Version = "dev"
	}
	if cfg.HeartbeatInterval <= 0 {
		cfg.HeartbeatInterval = 15 * time.Second
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 5 * time.Second
	}
	if cfg.AgentCode != "" && cfg.Token == "" {
		return runtimeConfig{}, fmt.Errorf("agent token is required when agent code is configured")
	}
	if cfg.AgentCode == "" && cfg.Token != "" {
		return runtimeConfig{}, fmt.Errorf("agent code is required when agent token is configured")
	}
	return cfg, nil
}

// readConfigFile 封装当前模块的业务处理逻辑。
func readConfigFile(path string) (fileConfig, error) {
	content, err := os.ReadFile(strings.TrimSpace(path))
	if err != nil {
		return fileConfig{}, fmt.Errorf("read config failed: %w", err)
	}
	var cfg fileConfig
	if err := yaml.Unmarshal(content, &cfg); err != nil {
		return fileConfig{}, fmt.Errorf("parse config failed: %w", err)
	}
	return cfg, nil
}

// bootstrapRuntimeCredentials 封装当前模块的业务处理逻辑。
func bootstrapRuntimeCredentials(client *http.Client, cfg runtimeConfig, hostname, hostIP, statePath string, localState agentLocalState) (runtimeConfig, error) {
	localState.MachineID = strings.TrimSpace(localState.MachineID)
	if localState.MachineID == "" {
		localState.MachineID = generateMachineID()
	}
	if cfg.AgentCode == "" {
		cfg.AgentCode = strings.TrimSpace(localState.AgentCode)
	}
	if cfg.Token == "" {
		cfg.Token = strings.TrimSpace(localState.Token)
	}
	if cfg.AgentCode != "" && cfg.Token != "" {
		if err := saveAgentLocalState(statePath, agentLocalState{
			MachineID: localState.MachineID,
			AgentCode: cfg.AgentCode,
			Token:     cfg.Token,
		}); err != nil {
			log.Printf("save local state failed: %v", err)
		}
		return cfg, nil
	}
	if cfg.RegistrationToken == "" {
		return runtimeConfig{}, fmt.Errorf("agent registration_token is required")
	}
	output, err := registerAgent(client, cfg, hostname, hostIP, localState.MachineID)
	if err != nil {
		return runtimeConfig{}, err
	}
	cfg.AgentCode = strings.TrimSpace(output.AgentCode)
	cfg.Token = strings.TrimSpace(output.Token)
	if err := saveAgentLocalState(statePath, agentLocalState{
		MachineID: localState.MachineID,
		AgentCode: cfg.AgentCode,
		Token:     cfg.Token,
	}); err != nil {
		log.Printf("save local state failed: %v", err)
	}
	return cfg, nil
}

// registerAgent 封装当前模块的业务处理逻辑。
func registerAgent(client *http.Client, cfg runtimeConfig, hostname, hostIP, machineID string) (*registerOutput, error) {
	var response registerResponse
	err := postJSON(client, cfg.BaseURL+"/agent/register", registerRequest{
		RegistrationToken: cfg.RegistrationToken,
		MachineID:         strings.TrimSpace(machineID),
		Name:              cfg.AgentName,
		EnvironmentCode:   cfg.EnvironmentCode,
		Hostname:          strings.TrimSpace(hostname),
		HostIP:            strings.TrimSpace(hostIP),
		AgentVersion:      cfg.Version,
		OS:                runtime.GOOS,
		Arch:              runtime.GOARCH,
		WorkDir:           cfg.WorkDir,
		Tags:              cfg.Tags,
	}, &response)
	if err != nil {
		return nil, err
	}
	if response.Data == nil || strings.TrimSpace(response.Data.AgentCode) == "" || strings.TrimSpace(response.Data.Token) == "" {
		return nil, fmt.Errorf("agent register returned empty runtime credentials")
	}
	return response.Data, nil
}

// agentStateFilePath 封装当前模块的业务处理逻辑。
func agentStateFilePath(workDir string) string {
	return filepath.Join(strings.TrimSpace(workDir), ".gos-agent", "state.json")
}

// loadAgentLocalState 封装当前模块的业务处理逻辑。
func loadAgentLocalState(path string) (agentLocalState, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return agentLocalState{}, nil
	}
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return agentLocalState{}, nil
		}
		return agentLocalState{}, err
	}
	var state agentLocalState
	if err := json.Unmarshal(content, &state); err != nil {
		return agentLocalState{}, err
	}
	state.MachineID = strings.TrimSpace(state.MachineID)
	state.AgentCode = strings.TrimSpace(state.AgentCode)
	state.Token = strings.TrimSpace(state.Token)
	return state, nil
}

// saveAgentLocalState 封装当前模块的业务处理逻辑。
func saveAgentLocalState(path string, state agentLocalState) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	state.MachineID = strings.TrimSpace(state.MachineID)
	state.AgentCode = strings.TrimSpace(state.AgentCode)
	state.Token = strings.TrimSpace(state.Token)
	state.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	content, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	content = append(content, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, content, 0o600)
}

// generateMachineID 封装当前模块的业务处理逻辑。
func generateMachineID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("machine-%d", time.Now().UTC().UnixNano())
	}
	return "machine-" + hex.EncodeToString(buf)
}

// sendHeartbeat 封装当前模块的业务处理逻辑。
func sendHeartbeat(client *http.Client, cfg runtimeConfig, hostname, hostIP string, state *runtimeState) error {
	payload := heartbeatRequest{
		AgentCode:    cfg.AgentCode,
		Token:        cfg.Token,
		Hostname:     strings.TrimSpace(hostname),
		HostIP:       strings.TrimSpace(hostIP),
		AgentVersion: cfg.Version,
		OS:           runtime.GOOS,
		Arch:         runtime.GOARCH,
		WorkDir:      cfg.WorkDir,
		Tags:         cfg.Tags,
	}
	snapshot := state.snapshot()
	payload.CurrentTaskID = snapshot.CurrentTaskID
	payload.CurrentTaskName = snapshot.CurrentTaskName
	payload.CurrentTaskType = snapshot.CurrentTaskType
	payload.CurrentTaskStarted = snapshot.CurrentTaskStarted
	payload.LastTaskStatus = snapshot.LastTaskStatus
	payload.LastTaskSummary = snapshot.LastTaskSummary
	payload.LastTaskFinishedAt = snapshot.LastTaskFinishedAt
	return postJSON(client, cfg.BaseURL+"/agent/heartbeat", payload, nil)
}

// pollTask 封装当前模块的业务处理逻辑。
func pollTask(client *http.Client, cfg runtimeConfig) (*taskOutput, error) {
	var response taskResponse
	err := postJSON(client, cfg.BaseURL+"/agent/tasks/poll", taskPollRequest{
		AgentCode: cfg.AgentCode,
		Token:     cfg.Token,
	}, &response)
	if err != nil {
		return nil, err
	}
	return response.Data, nil
}

// executeTask 封装当前模块的业务处理逻辑。
func executeTask(client *http.Client, cfg runtimeConfig, task *taskOutput, hostname, hostIP string, state *runtimeState) {
	if task == nil {
		return
	}
	startedAt := time.Now().UTC()
	state.markRunning(task, startedAt)
	_ = sendHeartbeat(client, cfg, hostname, hostIP, state)
	if err := postJSON(client, cfg.BaseURL+"/agent/tasks/"+task.ID+"/start", taskPollRequest{
		AgentCode: cfg.AgentCode,
		Token:     cfg.Token,
	}, nil); err != nil {
		finishedAt := time.Now().UTC()
		failureReason := "任务启动上报失败"
		state.markFinished("failed", failureReason, finishedAt)
		_ = sendHeartbeat(client, cfg, hostname, hostIP, state)
		_ = postJSON(client, cfg.BaseURL+"/agent/tasks/"+task.ID+"/finish", taskFinishRequest{
			AgentCode:     cfg.AgentCode,
			Token:         cfg.Token,
			Status:        "failed",
			ExitCode:      1,
			FailureReason: failureReason,
		}, nil)
		log.Printf("start task %s failed: %v", task.ID, err)
		return
	}

	status := "success"
	exitCode := 0
	stdoutText := ""
	stderrText := ""
	failureReason := ""

	execDir, err := prepareExecDir(cfg.WorkDir, task.ID)
	if err != nil {
		status = "failed"
		failureReason = err.Error()
	} else {
		switch strings.TrimSpace(task.TaskType) {
		case "file_distribution_task":
			deliveredPath, prepareErr := prepareFileDistributionTask(execDir, task)
			if prepareErr != nil {
				status = "failed"
				failureReason = prepareErr.Error()
			} else {
				stdoutText = fmt.Sprintf("file delivered to %s", deliveredPath)
			}
		default:
			scriptPath, prepareErr := prepareTaskScript(cfg.WorkDir, execDir, task)
			if prepareErr != nil {
				status = "failed"
				failureReason = prepareErr.Error()
			} else {
				stdoutText, stderrText, exitCode, err = runScript(execDir, task.ShellType, scriptPath, task.TimeoutSec)
				if err != nil {
					status = "failed"
					failureReason = err.Error()
				}
			}
		}
	}

	if status == "success" {
		state.markFinished("success", firstLine(stdoutText, "任务执行成功"), time.Now().UTC())
	} else {
		summary := failureReason
		if summary == "" {
			summary = firstLine(stderrText, "任务执行失败")
		}
		state.markFinished("failed", summary, time.Now().UTC())
	}
	_ = sendHeartbeat(client, cfg, hostname, hostIP, state)

	if err := postJSON(client, cfg.BaseURL+"/agent/tasks/"+task.ID+"/finish", taskFinishRequest{
		AgentCode:     cfg.AgentCode,
		Token:         cfg.Token,
		Status:        status,
		ExitCode:      exitCode,
		StdoutText:    trimText(stdoutText, 65535),
		StderrText:    trimText(stderrText, 65535),
		FailureReason: trimText(failureReason, 65535),
	}, nil); err != nil {
		log.Printf("finish task %s failed: %v", task.ID, err)
		return
	}
	log.Printf("task %s finished with status=%s exit=%d", task.ID, status, exitCode)
}

// prepareExecDir 封装当前模块的业务处理逻辑。
func prepareExecDir(baseWorkDir, taskID string) (string, error) {
	baseAbs, err := filepath.Abs(strings.TrimSpace(baseWorkDir))
	if err != nil {
		return "", err
	}
	execDir := filepath.Join(baseAbs, "tasks", strings.TrimSpace(taskID))
	execAbs, err := filepath.Abs(execDir)
	if err != nil {
		return "", err
	}
	if execAbs != baseAbs && !strings.HasPrefix(execAbs, baseAbs+string(os.PathSeparator)) {
		return "", fmt.Errorf("resolved exec dir is outside configured work dir")
	}
	if err := os.MkdirAll(execAbs, 0o755); err != nil {
		return "", err
	}
	return execAbs, nil
}

// runScript 封装当前模块的业务处理逻辑。
func runScript(execDir, shellType, scriptPath string, timeoutSec int) (stdoutText, stderrText string, exitCode int, err error) {
	shellPath := "/bin/sh"
	if strings.EqualFold(strings.TrimSpace(shellType), "bash") {
		shellPath = "/bin/bash"
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSec)*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, shellPath, scriptPath)
	cmd.Dir = execDir
	stdout := newLimitedBuffer(maxCapturedOutputBytes)
	stderr := newLimitedBuffer(maxCapturedOutputBytes)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()
	stdoutText = stdout.String()
	stderrText = stderr.String()
	if ctx.Err() == context.DeadlineExceeded {
		return stdoutText, stderrText, 124, fmt.Errorf("task execution timed out after %ds", timeoutSec)
	}
	if runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			return stdoutText, stderrText, exitErr.ExitCode(), fmt.Errorf("task execution failed with exit code %d", exitErr.ExitCode())
		}
		return stdoutText, stderrText, 1, runErr
	}
	return stdoutText, stderrText, 0, nil
}

// prepareTaskScript 封装当前模块的业务处理逻辑。
func prepareTaskScript(baseWorkDir, execDir string, task *taskOutput) (string, error) {
	if task == nil {
		return "", fmt.Errorf("task is required")
	}
	switch strings.TrimSpace(task.TaskType) {
	case "", "shell_task":
		rendered := renderTemplate(task.ScriptText, task.Variables)
		scriptPath := filepath.Join(execDir, scriptFileName(task.ShellType))
		if err := os.WriteFile(scriptPath, []byte(rendered), 0o700); err != nil {
			return "", err
		}
		return scriptPath, nil
	case "script_file_task":
		return prepareScriptFileTask(baseWorkDir, execDir, task)
	default:
		return "", fmt.Errorf("unsupported task type: %s", task.TaskType)
	}
}

// prepareScriptFileTask 封装当前模块的业务处理逻辑。
func prepareScriptFileTask(baseWorkDir, execDir string, task *taskOutput) (string, error) {
	scriptPathValue := strings.TrimSpace(task.ScriptPath)
	if scriptPathValue == "" {
		return "", fmt.Errorf("script file path is required")
	}
	if strings.TrimSpace(task.ScriptText) != "" {
		rendered := renderTemplate(task.ScriptText, task.Variables)
		fileName := filepath.Base(scriptPathValue)
		if fileName == "." || fileName == string(os.PathSeparator) || fileName == "" {
			fileName = scriptFileName(task.ShellType)
		}
		renderedPath := filepath.Join(execDir, fileName)
		if err := os.WriteFile(renderedPath, []byte(rendered), 0o700); err != nil {
			return "", err
		}
		return renderedPath, nil
	}
	baseAbs, err := filepath.Abs(strings.TrimSpace(baseWorkDir))
	if err != nil {
		return "", err
	}
	resolved := scriptPathValue
	if !filepath.IsAbs(resolved) {
		resolved = filepath.Join(baseAbs, resolved)
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return "", err
	}
	if resolved != baseAbs && !strings.HasPrefix(resolved, baseAbs+string(os.PathSeparator)) {
		return "", fmt.Errorf("script file path is outside configured work dir")
	}
	content, err := os.ReadFile(resolved)
	if err != nil {
		return "", err
	}
	rendered := renderTemplate(string(content), task.Variables)
	renderedPath := filepath.Join(execDir, filepath.Base(resolved))
	if err := os.WriteFile(renderedPath, []byte(rendered), 0o700); err != nil {
		return "", err
	}
	return renderedPath, nil
}

// prepareFileDistributionTask 封装当前模块的业务处理逻辑。
func prepareFileDistributionTask(execDir string, task *taskOutput) (string, error) {
	filePathValue := strings.TrimSpace(task.ScriptPath)
	if filePathValue == "" {
		return "", fmt.Errorf("file name is required")
	}
	if strings.TrimSpace(task.ScriptText) == "" {
		return "", fmt.Errorf("uploaded file content is required")
	}
	fileName := filepath.Base(filePathValue)
	if fileName == "." || fileName == string(os.PathSeparator) || fileName == "" {
		return "", fmt.Errorf("invalid file name")
	}
	targetPath := filepath.Join(execDir, fileName)
	rendered := renderTemplate(task.ScriptText, task.Variables)
	if err := os.WriteFile(targetPath, []byte(rendered), 0o644); err != nil {
		return "", err
	}
	return targetPath, nil
}

// postJSON 封装当前模块的业务处理逻辑。
func postJSON(client *http.Client, endpoint string, payload any, output any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	resp, err := client.Post(strings.TrimRight(strings.TrimSpace(endpoint), "/"), "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		responseBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return &statusError{Code: resp.StatusCode, Body: trimText(string(responseBody), 500)}
	}
	if output != nil {
		if err := json.NewDecoder(resp.Body).Decode(output); err != nil {
			return err
		}
	}
	return nil
}

type statusError struct {
	Code int
	Body string
}

// Error 封装当前模块的业务处理逻辑。
func (e *statusError) Error() string {
	if strings.TrimSpace(e.Body) == "" {
		return http.StatusText(e.Code)
	}
	return fmt.Sprintf("%s: %s", http.StatusText(e.Code), strings.TrimSpace(e.Body))
}

type limitedBuffer struct {
	max       int
	buf       bytes.Buffer
	truncated bool
}

// newLimitedBuffer 封装当前模块的业务处理逻辑。
func newLimitedBuffer(max int) limitedBuffer {
	return limitedBuffer{max: max}
}

// Write 写入处理结果或错误信息。
func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.max <= 0 {
		b.truncated = true
		return len(p), nil
	}
	remaining := b.max - b.buf.Len()
	if remaining > 0 {
		if len(p) <= remaining {
			_, _ = b.buf.Write(p)
		} else {
			_, _ = b.buf.Write(p[:remaining])
			b.truncated = true
		}
	} else {
		b.truncated = true
	}
	return len(p), nil
}

// String 封装当前模块的业务处理逻辑。
func (b *limitedBuffer) String() string {
	text := b.buf.String()
	if b.truncated {
		if text == "" {
			return "[output truncated]"
		}
		return text + "\n[output truncated]"
	}
	return text
}

// envOrDefault 封装当前模块的业务处理逻辑。
func envOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

// envDurationOrDefault 封装当前模块的业务处理逻辑。
func envDurationOrDefault(key string, fallback time.Duration) time.Duration {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		parsed, err := time.ParseDuration(value)
		if err == nil {
			return parsed
		}
	}
	return fallback
}

// currentDir 封装当前模块的业务处理逻辑。
func currentDir() string {
	dir, err := os.Getwd()
	if err != nil {
		return "."
	}
	return dir
}

// parseTags 解析输入内容并返回结构化结果。
func parseTags(raw string) []string {
	return normalizeTags(strings.Split(raw, ","))
}

// normalizeTags 标准化输入值，保证后续逻辑使用统一格式。
func normalizeTags(items []string) []string {
	result := make([]string, 0, len(items))
	seen := make(map[string]struct{})
	for _, item := range items {
		value := strings.TrimSpace(item)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

// renderTemplate 封装当前模块的业务处理逻辑。
func renderTemplate(script string, variables map[string]string) string {
	result := script
	for key, value := range variables {
		result = strings.ReplaceAll(result, "{"+key+"}", value)
	}
	return result
}

// firstLine 封装当前模块的业务处理逻辑。
func firstLine(value, fallback string) string {
	text := strings.TrimSpace(value)
	if text == "" {
		return fallback
	}
	lines := strings.Split(text, "\n")
	return trimText(strings.TrimSpace(lines[0]), 200)
}

// trimText 封装当前模块的业务处理逻辑。
func trimText(value string, max int) string {
	value = strings.TrimSpace(value)
	if max <= 0 || len(value) <= max {
		return value
	}
	return value[:max]
}

// scriptFileName 封装当前模块的业务处理逻辑。
func scriptFileName(shellType string) string {
	if strings.EqualFold(strings.TrimSpace(shellType), "bash") {
		return "run.bash"
	}
	return "run.sh"
}

// resolveAgentName 解析上下文数据，得到后续流程需要的结果。
func resolveAgentName(configuredName, hostname, hostIP string) string {
	if value := strings.TrimSpace(configuredName); value != "" {
		return value
	}
	hostPart := strings.TrimSpace(hostname)
	ipPart := formatAgentNameIP(hostIP)
	switch {
	case hostPart != "" && ipPart != "":
		return hostPart + "-" + ipPart
	case hostPart != "":
		return hostPart
	case ipPart != "":
		return "agent-" + ipPart
	default:
		return "agent"
	}
}

// formatAgentNameIP 封装当前模块的业务处理逻辑。
func formatAgentNameIP(hostIP string) string {
	value := strings.TrimSpace(hostIP)
	if value == "" {
		return ""
	}
	replacer := strings.NewReplacer(".", "-", ":", "-")
	value = replacer.Replace(value)
	for strings.Contains(value, "--") {
		value = strings.ReplaceAll(value, "--", "-")
	}
	value = strings.Trim(value, "-")
	return value
}

// discoverHostIP 封装当前模块的业务处理逻辑。
func discoverHostIP() string {
	interfaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, addrErr := iface.Addrs()
		if addrErr != nil {
			continue
		}
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok || ipNet.IP == nil || ipNet.IP.IsLoopback() {
				continue
			}
			if ip := ipNet.IP.To4(); ip != nil {
				return ip.String()
			}
		}
	}
	return ""
}
