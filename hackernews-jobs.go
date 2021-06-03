package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/smtp"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/spf13/viper"
)

const JOBS_CACHE_FILENAME string = "jobs-cache.json"

type JobsCache struct {
	Jobs map[string]int
}

type Job struct {
	text      string
	monthYear string
	commentLink string
}

func main() {
	viper.SetEnvPrefix("HNJ")
	viper.BindEnv("START_DATE")
	startDate, err := time.Parse("2006-01", viper.GetString("START_DATE"))
	if err != nil {
		log.Fatal(err)
	}

	res, err := http.Get("https://news.ycombinator.com/submitted?id=whoishiring")
	if err != nil {
		log.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		log.Fatalf("status code error: %d %s", res.StatusCode, res.Status)
	}

	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		log.Fatal(err)
	}

	links := make([]string, 0)

	doc.Find(".athing .storylink").Each(func(i int, s *goquery.Selection) {
		date := regexp.MustCompile(`\(.*\)$`).FindString(s.Text())
		date = date[1 : len(date)-1]
		storyDate, err := time.Parse("January 2006", date)
		if err != nil {
			log.Fatal(err)
		}

		isFreelancer, err := regexp.MatchString(`Freelancer`, s.Text())
		if err != nil {
			log.Fatal(err)
		}

		if storyDate.After(startDate) && isFreelancer {
			links = append(links, "https://news.ycombinator.com/"+s.AttrOr("href", ""))
		}
	})

	//Get jobs from links
	var jobs []Job
	for _, link := range links {
		jobsCache := getJobsCache()
		jobs = append(jobs, getJobs(link, jobsCache)...)
	}

	sendJobsEmail(jobs)

}

func sendJobsEmail(jobs []Job) {
	if len(jobs) == 0 {
		return
	}

	viper.BindEnv("MAIL_HOST")
	viper.BindEnv("MAIL_PORT")
	viper.BindEnv("MAIL_USERNAME")
	viper.BindEnv("MAIL_PASSWORD")
	viper.BindEnv("MAIL_FROM")
	viper.BindEnv("MAIL_TO")

	smtpHost := viper.GetString("MAIL_HOST")
	smtpPort := viper.GetString("MAIL_PORT")
	smtpUsername := viper.GetString("MAIL_USERNAME")
	smtpPassword := viper.GetString("MAIL_PASSWORD")
	mailFrom := viper.GetString("MAIL_FROM")
	mailTo := viper.GetString("MAIL_TO")

	var email strings.Builder

	email.WriteString("Subject: Hackernews Jobs [")
	email.WriteString(strconv.Itoa(len(jobs)))
	email.WriteString("]\r\n")
	email.WriteString("From: " + mailFrom + "\r\n")
	email.WriteString("To: " + mailTo + "\r\n")
	email.WriteString("MIME-version: 1.0;\nContent-Type: text/html; charset=\"UTF-8\";\r\n")
	email.WriteString("\r\n")

	for _, job := range jobs {
		email.WriteString(job.monthYear + " " + job.text)
		email.WriteString("<br><a href=\"https://news.ycombinator.com/" + job.commentLink + "\">View post</a>")
		email.WriteString("<br><br>")
	}
	
	auth := smtp.PlainAuth("", smtpUsername, smtpPassword, smtpHost)

	err := smtp.SendMail(smtpHost+":"+smtpPort, auth, mailFrom, []string{mailTo}, []byte(email.String()))
	if err != nil {
		fmt.Println(err)
	}

	log.Printf("%d job(s) emailed", len(jobs))
}

func getJobsCache() JobsCache {
	jsonFile, err := os.Open(JOBS_CACHE_FILENAME)
	if err != nil {
		return JobsCache{Jobs: map[string]int{}}
	}
	defer jsonFile.Close()

	byteValue, err := ioutil.ReadAll(jsonFile)
	if err != nil {
		log.Fatal(err)
	}

	var jobs JobsCache
	json.Unmarshal([]byte(byteValue), &jobs)

	return jobs
}

func saveJobsCache(jobsCache JobsCache) {
	b, err := json.Marshal(jobsCache)
	if err != nil {
		log.Fatal(err)
	}

	ioutil.WriteFile(JOBS_CACHE_FILENAME, b, 0644)
}

func getJobs(link string, jobsCache JobsCache) []Job {
	jobs := make([]Job, 0)

	res, err := http.Get(link)
	if err != nil {
		log.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		log.Fatalf("status code error: %d %s", res.StatusCode, res.Status)
	}

	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		log.Fatal(err)
	}

	pageTitle := doc.Find("table.fatitem .athing a.storylink").Text()
	re := regexp.MustCompile("\\(.+\\)")
	monthYear := re.FindString(pageTitle)

	doc.Find(".athing").Each(func(i int, s *goquery.Selection) {
		text, err := s.Find(".comment").Html()
		if err != nil {
			log.Fatal(err)
		}

		commentLink := s.Find(".age a").AttrOr("href", "")

		re := regexp.MustCompile("SEEKING.+FREELANCER")
		if re.MatchString(text) {
			//if comment has not being saved before
			_, found := jobsCache.Jobs[commentLink]
			if !found {
				jobs = append(jobs, Job{
					text:      strings.TrimSpace(text),
					monthYear: monthYear,
					commentLink: commentLink,
				})
				jobsCache.Jobs[commentLink] = 1
			}
		}
	})

	saveJobsCache(jobsCache)

	return jobs
}
