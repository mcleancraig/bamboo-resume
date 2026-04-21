package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/kennygrant/sanitize"
	"github.com/tidwall/gjson"
)

// Config holds the application configuration
type Config struct {
	SessionCookie string
	Subdomain     string
	OutputDir     string
	LocationISO   string // Added for location filtering
}

// Map for ISO codes to international phone prefixes
var countryPrefixes = map[string][]string{
	"US": {"+1", "001"},
	"GB": {"+44", "0044"},
	"CZ": {"+420", "00420"},
}

// Candidate represents a normalized applicant
type Candidate struct {
	ApplicantID    string
	FirstName      string
	LastName       string
	Rating         string
	ResumeFileID   string
	ResumeFileName string
	PositionID     string
	PositionName   string
	Phone          string
}

// Role represents a unique job position
type Role struct {
	ID   string
	Name string
}

func NewBambooClient(cfg Config) *BambooClient {
	return &BambooClient{
		config: cfg,
		http: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// BambooClient encapsulates the API logic and state
type BambooClient struct {
	http   *http.Client
	config Config
}

func (bc *BambooClient) executeRequest(method, url string) (*http.Response, error) {
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, err
	}
	req.AddCookie(&http.Cookie{
		Name:  "PHPSESSID",
		Value: bc.config.SessionCookie,
	})
	req.Header.Set("Accept", "application/json, text/javascript, */*; q=0.01")
	return bc.http.Do(req)
}

func (bc *BambooClient) FetchAllCandidates() ([]Candidate, error) {
	var candidates []Candidate
	url := fmt.Sprintf("https://%s.bamboohr.com/hiring/candidates?offset=0&limit=2000&sortOrder=DESC", bc.config.Subdomain)
	resp, err := bc.executeRequest("GET", url)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("returned non-200 status: %d", resp.StatusCode)
	}
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	jsonStr := string(bodyBytes)
	if !strings.HasPrefix(strings.TrimSpace(jsonStr), "{") {
		return nil, errors.New("received non-JSON response. Your session cookie may be expired or invalid")
	}
	ids := gjson.Get(jsonStr, "data.applicationsOrder")
	for _, id := range ids.Array() {
		appID := id.String()
		appPath := fmt.Sprintf("data.applications.%s", appID)
		candidateID := gjson.Get(jsonStr, appPath+".candidateId").String()
		jobID := gjson.Get(jsonStr, appPath+".jobOpeningId").String()
		candidates = append(candidates, Candidate{
			ApplicantID:    appID,
			FirstName:      gjson.Get(jsonStr, fmt.Sprintf("data.candidates.%s.firstName", candidateID)).String(),
			LastName:       gjson.Get(jsonStr, fmt.Sprintf("data.candidates.%s.lastName", candidateID)).String(),
			Rating:         gjson.Get(jsonStr, appPath+".rating").String(),
			ResumeFileID:   gjson.Get(jsonStr, appPath+".resumeFileId").String(),
			ResumeFileName: gjson.Get(jsonStr, appPath+".resumeFileName").String(),
			PositionID:     jobID,
			PositionName:   gjson.Get(jsonStr, fmt.Sprintf("data.jobOpenings.%s.name", jobID)).String(),
			Phone:          gjson.Get(jsonStr, fmt.Sprintf("data.candidates.%s.phone", candidateID)).String(),
		})
	}
	return candidates, nil
}

func (bc *BambooClient) DownloadResume(c Candidate) error {
	if c.ResumeFileID == "" || c.ResumeFileID == "0" || c.ResumeFileID == "null" {
		return errors.New("no resume file attached")
	}
	roleDir := filepath.Join(bc.config.OutputDir, "Shortlist", sanitize.BaseName(c.PositionName))
	os.MkdirAll(roleDir, 0755)
	safeRating := sanitize.BaseName(c.Rating)
	if safeRating == "" || safeRating == "null" {
		safeRating = "Unrated"
	}
	safeExt := filepath.Ext(c.ResumeFileName)
	if safeExt == "" {
		safeExt = ".pdf"
	}
	fileName := fmt.Sprintf("%s-%s-Rating_%s%s", sanitize.BaseName(c.FirstName), sanitize.BaseName(c.LastName), safeRating, safeExt)
	filePath := filepath.Join(roleDir, fileName)
	if _, err := os.Stat(filePath); err == nil {
		return errors.New("file already exists")
	}
	url := fmt.Sprintf("https://%s.bamboohr.com/files/download.php?id=%s", bc.config.Subdomain, c.ResumeFileID)
	resp, err := bc.executeRequest("GET", url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: %d", resp.StatusCode)
	}
	out, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer out.Close()
	io.Copy(out, resp.Body)
	return nil
}

func isTargetLocation(phone string, allowedISO string) bool {
	if strings.ToLower(allowedISO) == "all" {
		return true
	}

	var cleaned strings.Builder
	for _, ch := range phone {
		if (ch >= '0' && ch <= '9') || ch == '+' {
			cleaned.WriteRune(ch)
		}
	}
	cp := cleaned.String()

	targets := strings.Split(allowedISO, ",")
	for _, target := range targets {
		iso := strings.ToUpper(strings.TrimSpace(target))
		prefixes, exists := countryPrefixes[iso]
		if !exists {
			continue
		}

		for _, p := range prefixes {
			if strings.HasPrefix(cp, p) {
				return true
			}
		}

		// Local US number check (10 digits, no prefix)
		if iso == "US" && len(cp) == 10 && !strings.HasPrefix(cp, "+") {
			return true
		}
	}

	return false
}

func InteractiveRolePicker(candidates []Candidate) []string {
	roleMap := make(map[string]string)
	for _, c := range candidates {
		if c.PositionID != "" && c.PositionID != "null" && c.PositionName != "" {
			roleMap[c.PositionID] = c.PositionName
		}
	}
	var uniqueRoles []Role
	for id, name := range roleMap {
		uniqueRoles = append(uniqueRoles, Role{ID: id, Name: name})
	}
	if len(uniqueRoles) == 0 {
		fmt.Println("No active roles found.")
		os.Exit(0)
	}
	fmt.Println("\n=== Active Roles ===")
	for i, r := range uniqueRoles {
		fmt.Printf("[%d] %s (ID: %s)\n", i+1, r.Name, r.ID)
	}
	fmt.Print("\nEnter numbers (comma-separated) or 0 for ALL: ")
	input, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	input = strings.TrimSpace(input)
	if input == "0" || input == "" {
		return []string{}
	}
	var selected []string
	for _, choice := range strings.Split(input, ",") {
		index, _ := strconv.Atoi(strings.TrimSpace(choice))
		if index > 0 && index <= len(uniqueRoles) {
			selected = append(selected, uniqueRoles[index-1].ID)
		}
	}
	return selected
}

func main() {
	// Custom Help Output
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "BambooHR Resume Downloader\n\n")
		fmt.Fprintf(os.Stderr, "Usage: ./bamboo -c <cookie> -s <subdomain> -l <US|GB|CZ|all> [options]\n\n")
		fmt.Fprintf(os.Stderr, "Required Flags:\n")
		fmt.Fprintf(os.Stderr, "  -c string     Your PHPSESSID cookie value (get from your Browser session).\n")
		fmt.Fprintf(os.Stderr, "  -s string     Your BambooHR subdomain.\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		fmt.Fprintf(os.Stderr, "  -d string     Output directory (default './BambooResumes')\n")
		fmt.Fprintf(os.Stderr, "  -roles string Comma-separated Position IDs (skips interactive menu)\n\n")
	}

	cookieFlag := flag.String("c", "", "PHPSESSID Cookie")
	rolesFlag := flag.String("roles", "", "Position IDs")
	sd := flag.String("s", "", "Subdomain")
	locFlag := flag.String("l", "", "Location ISO codes")
	outDir := flag.String("d", "./BambooResumes", "Output directory")
	flag.Parse()

	if *sd == "" || *locFlag == "" || (*cookieFlag == "" && os.Getenv("BAMBOO_SESSION_COOKIE") == "") {
		flag.Usage()
		os.Exit(1)
	}

	cookie := *cookieFlag
	if cookie == "" {
		cookie = os.Getenv("BAMBOO_SESSION_COOKIE")
	}

	logFile, _ := os.OpenFile("bamboo.log", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0600)
	defer logFile.Close()
	log.SetOutput(logFile)

	absDir, _ := filepath.Abs(*outDir)
	client := NewBambooClient(Config{
		SessionCookie: cookie,
		Subdomain:     *sd,
		OutputDir:     absDir,
		LocationISO:   *locFlag,
	})

	candidates, err := client.FetchAllCandidates()
	if err != nil {
		log.Fatalf("Error: %v", err)
	}

	var targetRoles []string
	if *rolesFlag != "" {
		for _, p := range strings.Split(*rolesFlag, ",") {
			targetRoles = append(targetRoles, strings.TrimSpace(p))
		}
	} else {
		targetRoles = InteractiveRolePicker(candidates)
	}

	var shortlisted []Candidate
	for _, c := range candidates {
		match := false
		if len(targetRoles) == 0 {
			match = true
		}
		for _, tr := range targetRoles {
			if c.PositionID == tr {
				match = true
				break
			}
		}
		if match && isTargetLocation(c.Phone, client.config.LocationISO) {
			shortlisted = append(shortlisted, c)
		}
	}

	fmt.Printf("\nFound %d valid candidates matching filters. Downloading...\n", len(shortlisted))
	downloaded := 0
	for _, c := range shortlisted {
		if err := client.DownloadResume(c); err == nil {
			downloaded++
			fmt.Printf("[%d/%d] Downloaded: %s %s\n", downloaded, len(shortlisted), c.FirstName, c.LastName)
		}
	}
	fmt.Printf("\nDone! Resumes saved to %s/Shortlist\n", absDir)
}
