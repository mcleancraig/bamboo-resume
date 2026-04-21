# BambooHR Resume Downloader

A lightweight, fast Go utility designed to fetch and download applicant resumes from BambooHR.

Because BambooHR's standard API lacks comprehensive endpoint support for downloading applicant files—and traditional scraping is often blocked by enterprise Single Sign-On (SSO) or Multi-Factor Authentication (MFA)—this tool utilizes your active browser session (`PHPSESSID`) to authenticate and securely interact with the backend API.

## Features

* **SSO & MFA Compatible:** Bypasses complex login flows by utilizing your active session cookie.
* **Interactive Role Picker:** Automatically detects active job postings and lets you select which roles to download candidates for.
* **Status Filtering:** Only download candidates in specific stages (e.g., `New`, `Reviewed`, `Not a Fit`).
* **Location Filtering:** Filter candidates by international dialing codes using ISO country codes (e.g., `US,GB,CZ`).
* **Auto-Organization:** Automatically creates folders for each role and names files neatly (e.g., `FirstName-LastName-Rating_4.pdf`).

## Prerequisites

* [Go](https://go.dev/dl/) (1.18 or higher)
* A valid BambooHR account with permissions to view and download candidate resumes.

## Installation

1. Clone the repository:  
   `git clone https://github.com/yourusername/bamboohr-resume-downloader.git`  
   `cd bamboohr-resume-downloader`

2. Install the required Go dependencies:  
   `go get github.com/kennygrant/sanitize`  
   `go get github.com/tidwall/gjson`

3. Build the executable:  
   `go build -o bamboo bamboo.go`

## How to Get Your Session Cookie

To use this tool, you need your active BambooHR session cookie (`PHPSESSID`).

1. Log in to your BambooHR account in your web browser.
2. Open **Developer Tools** (Right-click -> Inspect, or `F12`).
3. Go to the **Application** tab (Chrome/Edge) or **Storage** tab (Firefox).
4. Expand the **Cookies** section on the left sidebar and select your BambooHR URL.
5. Find the row named `PHPSESSID`. Copy the **Value**.

## Usage

You can pass your cookie via the command line or set it as an environment variable (recommended for security).

**Using Environment Variable (Recommended):**  
`export BAMBOO_SESSION_COOKIE="your_phpsessid_value_here"`  
`./bamboo -s mycompany`

**Using Command Line Flags:**  
`./bamboo -c "your_phpsessid_value_here" -s mycompany`

### Flags & Options

| Flag | Default | Description |
| :--- | :--- | :--- |
| `-s` | *Required* | Your BambooHR subdomain (e.g., for `mycompany.bamboohr.com`, use `mycompany`). |
| `-c` | *Required** | Your `PHPSESSID` cookie. *(Optional if `BAMBOO_SESSION_COOKIE` is set).* |
| `-status` | `all` | Filter by candidate status (e.g., `New`, `Not a Fit`). Case-insensitive. |
| `-l` | `all` | Filter by location using comma-separated ISO country codes (e.g., `US,GB,CZ`). |
| `-roles` | *None* | Comma-separated Position IDs to download (skips the interactive menu). |
| `-d` | `./BambooResumes` | The output directory where the folders and resumes will be saved. |

### Examples

**1. Interactive Mode (Default)**
Fetch all "New" candidates from the US and UK. The script will present a menu asking which job roles you want to download.  
`./bamboo -s mycompany -l "US,GB" -status "new"`

**2. Fully Automated (No Menu)**
Download all candidates currently marked as "Reviewed" for Position IDs `102` and `105`, saving them to a custom folder.  
`./bamboo -s mycompany -roles "102,105" -status "reviewed" -d "/Users/Shared/HR_Downloads"`

## Configuring Custom Candidate Statuses

BambooHR allows companies to create custom applicant tracking stages. If your script fails to find candidates when using the `-status` flag, you may need to map your custom status ID in the code.

1. Open `bamboo.go`.
2. Locate the `statusMap` variable near the top of the file:
   var statusMap = map[string]string{
       "new":       "1",
       "not a fit": "4",
       // Add your custom statuses here
       "phone screen": "12",
   }
3. Add your custom status (the key *must* be completely lowercase) and its corresponding BambooHR ID. Rebuild the binary.

*(Tip: If you don't want to edit the code, you can simply pass the raw ID to the flag, e.g., `-status 12`).*

## Troubleshooting

* **"received non-JSON response. Your session cookie may be expired or invalid"**  
  Your `PHPSESSID` cookie has expired or was copied incorrectly. Log out of BambooHR, log back in, and grab the new cookie value.

* **"No active roles found"**  
  Your account may not have permission to view active job openings, or your cookie is invalid.

* **0 Candidates Downloaded**  
  Check your `-status` and `-l` filters. If a candidate doesn't have a phone number on file, they will be skipped if you have a strict location filter applied. Use `-l all` to bypass location filtering.

## Disclaimer

This tool interacts with internal BambooHR endpoints. It is intended for authorized administrative use only. Always ensure you are complying with your organization's data privacy, PII handling, and API usage policies.
