package main

import (
	"bytes"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/bluele/slack"
	"github.com/globalsign/mgo"
	"github.com/globalsign/mgo/bson"
	"github.com/qmerce/adstxt"
)

var (
	mongo        = flag.String("mongo-url", os.Getenv("MONGODB_URL"), "mongodb url")
	dbSeed       = flag.Bool("db-seed", false, "run db seed of ads.txt file")
	slackToken   = flag.String("slack-token", os.Getenv("SLACK_TOKEN"), "slack api token")
	slackChannel = flag.String("slack-channel", "ads-txt-crawler", "slack api token")
	bulk         = flag.Int("bulk", 100, "bulk size to crawl")
)

const (
	proto = "https://"
	path  = "/ads.txt"
)

var client *http.Client

func init() {
	client = &http.Client{Timeout: 5 * time.Second}
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
	dialInfo, err := mgo.ParseURL(*mongo)
	if err != nil {
		log.Fatalf("could not connect to db: %v", err)
	}

	// dialInfo.DialServer = func(addr *mgo.ServerAddr) (net.Conn, error) {
	// 	conn, err := tls.Dial("tcp", addr.String(), &tls.Config{})
	// 	return conn, err
	// }

	db, err := mgo.DialWithInfo(dialInfo)
	if err != nil {
		log.Fatalf("could not connect to db: %v", err)
	}
	defer db.Close()

	c := db.DB("").C("adstxt")
	if *dbSeed {
		if err = seed(c); err != nil {
			log.Fatalf("failed seeding: %v", err)
		}
		os.Exit(0)
	}

	var doc AdsTXTFile
	if err := c.Find(bson.M{"_id": "ads_txt_file"}).One(&doc); err != nil {
		log.Fatalf("could not get adstxt from db: %v", err)
	}

	// convert to io.Reader
	b := bytes.NewBufferString(doc.File)
	ads, err := adstxt.Parse(b)
	if err != nil {
		log.Fatalf("could not get ads.txt from db: %v", err)
	}

	// var wg sync.WaitGroup
	// res := make(chan string, len(doc.Domains))
	var res []string
	for i, domain := range doc.Domains {
		// wg.Add(len(ads))
		go find(domain, ads, res)
		if i%*bulk == 0 {
			time.Sleep(30 * time.Second)
		}
	}

	// var out []string
	// go func() {
	// 	for msg := range res {
	// 		out = append(out, msg)
	// wg.Done()
	// }
	// }()
	// wg.Wait()
	send(*slackToken, *slackChannel, strings.Join(res, "\n"))
}

// func find(domain string, src []adstxt.Record, res chan string) {
func find(domain string, src []adstxt.Record, res []string) {
	dst, err := GetDomain(domain)
	if err != nil {
		log.Printf("could not crawl domain %s: %v", domain, err)
		// res <- fmt.Sprintf("%s, %q, failed", domain, err)
		res = append(res, fmt.Sprintf("%s, %q, failed", domain, err))
	}
	for _, v := range src {
		if match(dst, v) {
			// res <- fmt.Sprintf("%s, \"%s, %s\", 1", domain, v.ExchangeDomain, v.PublisherAccountID)
			res = append(res, fmt.Sprintf("%s, \"%s, %s\", 1", domain, v.ExchangeDomain, v.PublisherAccountID))
			continue
		}
		// res <- fmt.Sprintf("%s, \"%s, %s\", 0", domain, v.ExchangeDomain, v.PublisherAccountID)
		res = append(res, fmt.Sprintf("%s, \"%s, %s\", 0", domain, v.ExchangeDomain, v.PublisherAccountID))
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
