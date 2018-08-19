package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/bluele/slack"
	"github.com/mongodb/mongo-go-driver/mongo"
	"github.com/mongodb/mongo-go-driver/mongo/clientopt"
	"github.com/qmerce/adstxt"
)

var (
	mongoURL     = flag.String("mongo-url", os.Getenv("MONGODB_URL"), "mongodb url")
	dbSeed       = flag.Bool("db-seed", false, "run db seed of ads.txt file")
	slackToken   = flag.String("slack-token", os.Getenv("SLACK_TOKEN"), "slack api token")
	slackChannel = flag.String("slack-channel", "ads-txt-crawler", "slack channel")
	bulk         = flag.Int("bulk", 100, "bulk size to crawl")
)

const (
	proto = "https://"
	path  = "/ads.txt"
)

var client *http.Client

func init() {
	client = &http.Client{Timeout: 10 * time.Second}
}

// GetDomain crawls URL and returns a slice of Records
func GetDomain(domain string) ([]adstxt.Record, error) {
	resp, err := client.Get(proto + domain + path)
	if err != nil {
		resp, err = client.Get("http://" + domain + path)
		if err != nil {
			return nil, err
		}
	}
	defer resp.Body.Close()
	return adstxt.Parse(resp.Body)
}

// AdsTXTFile represent MongoDB BSON record
type AdsTXTFile struct {
	ID      string   `bson:"_id"`
	File    string   `bson:"file"`
	Domains []string `bson:"domains"`
}

func main() {
	flag.Parse()

	ctx := context.Background()
	db, err := mongo.Connect(ctx, *mongoURL, clientopt.ServerSelectionTimeout(5*time.Minute))
	if err != nil {
		log.Fatalf("could not connect to db: %v", err)
	}
	defer db.Disconnect(ctx)

	c := db.Database("adstxt").Collection("adstxt")
	if *dbSeed {
		if err = seed(ctx, c); err != nil {
			log.Fatalf("failed seeding: %v", err)
		}
		os.Exit(0)
	}

	var doc AdsTXTFile
	if err := c.FindOne(ctx, map[string]string{"_id": "ads_txt_file"}).Decode(&doc); err != nil {
		log.Fatalf("could not get adstxt from db: %v", err)
	}

	// convert to io.Reader
	b := bytes.NewBufferString(doc.File)
	ads, err := adstxt.Parse(b)
	if err != nil {
		log.Fatalf("could not get ads.txt from db: %v", err)
	}

	log.Println("launch worker")
	res := []string{"domain, exchange, status"}
	ch := make(chan string, len(doc.Domains))
	go func() {
		for msg := range ch {
			res = append(res, msg)
		}
	}()

	log.Println("scrape ads.txt from domain list")
	wg := new(sync.WaitGroup)
	for i, domain := range doc.Domains {
		wg.Add(1)
		go find(domain, ads, ch, wg)
		if i%*bulk == 0 {
			time.Sleep(30 * time.Second)
		}
	}
	wg.Wait()
	close(ch)

	file := strings.Join(res, "\n")
	log.Println("send result csv to slack")
	if err = send(*slackToken, *slackChannel, file); err != nil {
		log.Fatalf("could not upload file to slack: %v", err)
	}
}

// func find(domain string, src []adstxt.Record, res chan string) {
func find(domain string, src []adstxt.Record, ch chan string, wg *sync.WaitGroup) {
	defer wg.Done()
	dst, err := GetDomain(domain)
	if err != nil {
		log.Printf("error: %v", err)
		ch <- fmt.Sprintf("%q, %q, %q", domain, err, "failed")
	}
	for _, v := range src {
		exchange := fmt.Sprintf("%s, %s", v.ExchangeDomain, v.PublisherAccountID)
		if match(dst, v) {
			ch <- fmt.Sprintf("%q, %q, %q", domain, exchange, "1")
			continue
		}
		ch <- fmt.Sprintf("%q, %q, %q", domain, exchange, "0")
	}
}

func match(dst []adstxt.Record, r adstxt.Record) bool {
	for _, v := range dst {
		if v.ExchangeDomain == r.ExchangeDomain && v.PublisherAccountID == r.PublisherAccountID {
			return true
		}
	}
	return false
}

func send(token, channel, content string) error {
	api := slack.New(token)
	ch, err := api.FindChannelByName(channel)
	if err != nil {
		return err
	}
	_, err = api.FilesUpload(&slack.FilesUploadOpt{
		Title:    "Today's report",
		Content:  content,
		Filetype: "csv",
		Filename: "adstxt-report.csv",
		Channels: []string{ch.Id},
	})
	return err
}
