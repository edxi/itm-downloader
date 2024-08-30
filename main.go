package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

const (
	loginURL   = "https://cn2-edge.api.xijiabrainmap.com/api/user/login"
	jobListURL = "https://cn2-edge.api.xijiabrainmap.com/api/job"
	archiveURL = "https://cn2-edge.api.xijiabrainmap.com/api/job/%s/archive"
)

var (
	facility = flag.String("facility", "", "登陆所属facility \n 对应环境变量FACILITY")
	username = flag.String("username", "", "登陆用户名 \n 对应环境变量USERNAME")
	password = flag.String("password", "", "登陆密码 \n 对应环境变量PASSWORD")
	outDir   = flag.String("outDir", "", "下载目录 \n 对应环境变量OUTDIR")
	logfile  = flag.String("logfile", "", "日志文件，缺省不填写即输出到控制台。 \n 对应环境变量LOGFILE")
)

type LoginPayload struct {
	Facility string `json:"facility"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type Job struct {
	JobID     string `json:"job_id"`
	PatientID string `json:"patient_id"`
}

func main() {
	flag.StringVar(facility, "f", "", "facility参数的短写")
	flag.StringVar(username, "u", "", "username参数的短写")
	flag.StringVar(password, "p", "", "password参数的短写")
	flag.StringVar(outDir, "o", "./", "outDir参数的短写")
	flag.StringVar(logfile, "l", "", "logfile参数的短写")

	// 解析命令行参数
	flag.Parse()

	// 载入.env
	godotenv.Load(".env")

	if (*facility == "" && os.Getenv("FACILITY") == "") ||
		(*username == "" && os.Getenv("USERNAME") == "") ||
		(*password == "" && os.Getenv("PASSWORD") == "") {
		flag.Usage()
		os.Exit(1)
	}

	if *facility == "" {
		*facility = os.Getenv("FACILITY")
	}
	if *username == "" {
		*username = os.Getenv("USERNAME")
	}
	if *password == "" {
		*password = os.Getenv("PASSWORD")
	}

	if *logfile != "" || ((func() bool { *logfile = os.Getenv("LOGFILE"); return *logfile != "" })()) {
		file, err := os.OpenFile(*logfile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			log.Fatalln("无法创建日志文件")
		}
		defer file.Close()

		log.SetOutput(file)
	}

	// Create HTTP client
	client := &http.Client{}

	// Login
	cookies, err := login(client)
	if err != nil {
		log.Fatalf("Login failed: %v", err)
	}

	// List jobs
	jobs, err := listJobs(client, cookies)
	if err != nil {
		log.Fatalf("Failed to list jobs: %v", err)
	}

	// Create output directory
	if err := os.MkdirAll(*outDir, 0755); err != nil {
		log.Fatalf("Failed to create output directory: %v", err)
	}

	// Download archives for each job
	for _, job := range jobs {
		if err := downloadArchive(client, cookies, job); err != nil {
			log.Printf("Failed to download archive for job %s: %v", job.JobID, err)
		}
	}
}

func login(client *http.Client) ([]*http.Cookie, error) {
	payload := LoginPayload{
		Facility: *facility,
		Username: *username,
		Password: *password,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal login payload: %w", err)
	}

	req, err := http.NewRequest("POST", loginURL, strings.NewReader(string(payloadBytes)))
	if err != nil {
		return nil, fmt.Errorf("failed to create login request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("login request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("login failed with status code: %d", resp.StatusCode)
	}

	log.Println("INFO: ", *username, "成功登陆到", *facility)
	return resp.Cookies(), nil
}

func listJobs(client *http.Client, cookies []*http.Cookie) ([]Job, error) {
	req, err := http.NewRequest("GET", jobListURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create job list request: %w", err)
	}

	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("job list request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("job list request failed with status code: %d", resp.StatusCode)
	}

	var jobs []Job
	if err := json.NewDecoder(resp.Body).Decode(&jobs); err != nil {
		return nil, fmt.Errorf("failed to decode job list response: %w", err)
	}

	return jobs, nil
}

func downloadArchive(client *http.Client, cookies []*http.Cookie, job Job) error {
	log.Printf("Starting downloading archive for job %s, patient ID is %s ", job.JobID, job.PatientID)
	url := fmt.Sprintf(archiveURL, job.JobID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create archive download request: %w", err)
	}

	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("archive download request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("archive download failed with status code: %d", resp.StatusCode)
	}

	fileName := fmt.Sprintf("%s_%s_%s.zip", job.JobID, job.PatientID, time.Now().Format("20060102_150405"))
	filePath := filepath.Join(*outDir, fileName)

	out, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to write archive data: %w", err)
	}

	log.Printf("Successfully downloaded archive for job %s to %s", job.JobID, filePath)
	return nil
}
