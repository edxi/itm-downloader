package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/joho/godotenv"
)

const (
	loginURL   = "https://infinitome.cn.xijiabrainmap.com/api/v1/auth/login"
	jobListURL = "https://infinitome.cn.xijiabrainmap.com/api/v1/infinitome-sessions/%s/latest"
	archiveURL = "https://infinitome.cn.xijiabrainmap.com/api/v1/jobs/%s/%s/files"
)

var (
	facility = flag.String("facility", "", "登录所属facility \n 对应环境变量FACILITY")
	username = flag.String("username", "", "登录用户名 \n 对应环境变量USERNAME")
	password = flag.String("password", "", "登录密码 \n 对应环境变量PASSWORD")
	pin      = flag.String("pin", "", "登录PIN \n 对应环境变量PIN")
	outDir   = flag.String("outDir", "", "下载目录 \n 对应环境变量OUTDIR")
	logfile  = flag.String("logfile", "", "日志文件，缺省不填写即输出到控制台。 \n 对应环境变量LOGFILE")
)

type LoginPayload struct {
	FacilityCode string `json:"facilityCode"`
	Username     string `json:"username"`
	Password     string `json:"password"`
	Pin          string `json:"pin"`
}

type LoginResponse struct {
	Facility struct {
		ID string `json:"id"`
	} `json:"facility"`
}

type SessionResponse struct {
	Session map[string]struct {
		ImagingSessionUid string `json:"imagingSessionUid"`
		Job struct {
			ID        string    `json:"id"`
			UpdatedAt time.Time `json:"updatedAt"`
		} `json:"job"`
	} `json:"session"`
}

type Job struct {
	JobID     string
	PatientID string
}

func main() {
	flag.StringVar(facility, "f", "", "facility参数的短写")
	flag.StringVar(username, "u", "", "username参数的短写")
	flag.StringVar(password, "p", "", "password参数的短写")
	flag.StringVar(pin, "n", "", "pin参数的短写")
	flag.StringVar(outDir, "o", "./", "outDir参数的短写")
	flag.StringVar(logfile, "l", "", "logfile参数的短写")

	// 解析命令行参数
	flag.Parse()

	// 载入.env
	godotenv.Load(".env")

	if (*facility == "" && os.Getenv("FACILITY") == "") ||
		(*username == "" && os.Getenv("USERNAME") == "") ||
		(*password == "" && os.Getenv("PASSWORD") == "") ||
		(*pin == "" && os.Getenv("PIN") == "") {
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
	if *pin == "" {
		*pin = os.Getenv("PIN")
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
	cookies, facilityID, err := login(client)
	if err != nil {
		log.Fatalf("登录失败: %v", err)
	}

	// 列出任务
	jobs, err := listJobs(client, cookies, facilityID)
	if err != nil {
		log.Fatalf("获取任务列表失败: %v", err)
	}

	// 创建输出目录
	if err := os.MkdirAll(*outDir, 0755); err != nil {
		log.Fatalf("创建输出目录失败: %v", err)
	}

	// Download archives for each job
	for _, job := range jobs {
		if err := downloadArchive(client, cookies, facilityID, job); err != nil {
			log.Printf("下载任务 %s 的存档失败: %v", job.JobID, err)
		}
	}
}

func login(client *http.Client) ([]*http.Cookie, string, error) {
	payload := LoginPayload{
		FacilityCode: *facility,
		Username:     *username,
		Password:     *password,
		Pin:          *pin, // 使用从命令行或环境变量获取的 PIN
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, "", fmt.Errorf("无法序列化登录 payload: %w", err)
	}

	req, err := http.NewRequest("POST", loginURL, bytes.NewBuffer(payloadBytes))
	if err != nil {
		return nil, "", fmt.Errorf("无法创建登录请求: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("登录请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("登录失败，状态码: %d", resp.StatusCode)
	}

	var loginResp LoginResponse
	if err := json.NewDecoder(resp.Body).Decode(&loginResp); err != nil {
		return nil, "", fmt.Errorf("无法解析登录响应: %w", err)
	}

	facilityID := loginResp.Facility.ID

	log.Println("信息: ", *username, "成功登录到", *facility)
	return resp.Cookies(), facilityID, nil
}

func listJobs(client *http.Client, cookies []*http.Cookie, facilityID string) ([]Job, error) {
	url := fmt.Sprintf(jobListURL, facilityID)
	payload := map[string]int{"limit": 50} // API限制了最多50

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("无法序列化任务列表 payload: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(payloadBytes))
	if err != nil {
		return nil, fmt.Errorf("创建任务列表请求失败: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("任务列表请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("任务列表请求失败，状态码: %d", resp.StatusCode)
	}

	var sessionResp SessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&sessionResp); err != nil {
		return nil, fmt.Errorf("无法解析任务列表响应: %w", err)
	}

	var jobs []Job
	for _, session := range sessionResp.Session {
		patientID := fmt.Sprintf("%s_%s", session.ImagingSessionUid, session.Job.UpdatedAt.Format("20060102_150405"))
		jobs = append(jobs, Job{
			JobID:     session.Job.ID,
			PatientID: patientID,
		})
	}

	return jobs, nil
}

func downloadArchive(client *http.Client, cookies []*http.Cookie, facilityID string, job Job) error {
	log.Printf("开始下载任务 %s 的存档，患者 ID 为 %s", job.JobID, job.PatientID)
	url := fmt.Sprintf(archiveURL, facilityID, job.JobID)
	log.Println(url)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("创建存档下载请求失败: %w", err)
	}

	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("存档下载请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("存档下载失败，状态码: %d", resp.StatusCode)
	}

	fileName := fmt.Sprintf("%s_%s.zip", job.JobID, job.PatientID)
	filePath := filepath.Join(*outDir, fileName)

	out, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("创建输出文件失败: %w", err)
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return fmt.Errorf("写入存档数据失败: %w", err)
	}

	log.Printf("成功下载任务 %s 的存档到 %s", job.JobID, filePath)
	return nil
}
