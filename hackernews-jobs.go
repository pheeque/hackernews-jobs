package main

import (
	"fmt"
	"log"
	"net/http"
	"net/smtp"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/robfig/cron/v3"
	"github.com/spf13/viper"
	"github.com/xujiajun/nutsdb"
)

const JOBS_CACHE_FILENAME string = "jobs-cache.json"

type JobsCache struct {
	Jobs map[string]int
}

type Job struct {
	text        string
	monthYear   string
	commentLink string
}

func main() {
	log.SetOutput(os.Stdout)

	log.Print("started")
	c := cron.New()
	c.AddFunc("@every 1h", func() {
		run()

		alertHealthchecks()
	})
	c.Start()

	select {}
}

func alertHealthchecks() {
	viper.BindEnv("HEALTHCHECKS_ENDPOINT")

	url := viper.GetString("HEALTHCHECKS_ENDPOINT")

	http.Get(url)
}

func run() {
	viper.SetEnvPrefix("BOT")

	opt := nutsdb.DefaultOptions
	opt.Dir = "bin"
	db, err := nutsdb.Open(opt)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

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

	var selector = ".athing .titlelink"
	if doc.Find(selector).Length() == 0 {
		sendEmail("Hackernews bot selectors issue", "submissions list is empty")
		return
	}

	doc.Find(selector).Each(func(i int, s *goquery.Selection) {
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
		jobs = append(jobs, getJobs(link, db)...)
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

	email.WriteString("Subject: New job(s) [")
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

func getJobs(link string, db *nutsdb.DB) []Job {
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

	pageTitle := doc.Find("table.fatitem .athing a.titlelink").Text()
	re := regexp.MustCompile(`\(.+\)`)
	monthYear := re.FindString(pageTitle)


	var selector = ".athing .comment"
	if doc.Find(selector).Length() == 0 {
		sendEmail("Hackernews bot selectors issue", "page comments empty")
		return jobs //empty
	}

	doc.Find(".athing").Each(func(i int, s *goquery.Selection) {
		text, err := s.Find(".comment").Html()
		if err != nil {
			log.Fatal(err)
		}

		commentLink := s.Find(".age a").AttrOr("href", "")

		re := regexp.MustCompile("SEEKING.+FREELANCER")
		if re.MatchString(text) {
			//if comment has not being saved before
			if !inCache(db, commentLink) {
				jobs = append(jobs, Job{
					text:        strings.TrimSpace(text),
					monthYear:   monthYear,
					commentLink: commentLink,
				})
				addToCache(db, commentLink)
			}
		}
	})

	return jobs
}

func addToCache(db *nutsdb.DB, link string) {
	if err := db.Update(
		func(tx *nutsdb.Tx) error {
			bucket := "posts"
			key := []byte("links")
			return tx.SAdd(bucket, key, []byte(link))
		}); err != nil {
			log.Print(err)
	}
}

func inCache(db *nutsdb.DB, link string) bool {
	result := false
	db.View(
		func(tx *nutsdb.Tx) error {
			if ok, err := tx.SIsMember("posts", []byte("links"), []byte(link)); err != nil {
				result = false
				return err
			} else {
				result = ok
			}
			return nil
		});

	return result
}

func sendEmail(subject string, content string) {
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

	email.WriteString("Subject: ")
	email.WriteString(subject)
	email.WriteString("\r\n")
	email.WriteString("From: " + mailFrom + "\r\n")
	email.WriteString("To: " + mailTo + "\r\n")
	email.WriteString("MIME-version: 1.0;\nContent-Type: text/html; charset=\"UTF-8\";\r\n")
	email.WriteString("\r\n")
	
	email.WriteString(content)

	auth := smtp.PlainAuth("", smtpUsername, smtpPassword, smtpHost)

	err := smtp.SendMail(smtpHost+":"+smtpPort, auth, mailFrom, []string{mailTo}, []byte(email.String()))
	if err != nil {
		fmt.Println(err)
	}
}