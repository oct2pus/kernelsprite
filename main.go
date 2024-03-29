package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/mattn/go-mastodon"
	bolt "go.etcd.io/bbolt"
)

var (
	posts     = []byte("posts")
	followers = []byte("followers")
)

func main() {
	// get api key
	buf := bufio.NewReader(os.Stdin)
	app, err := mastodon.RegisterApp(context.Background(), &mastodon.AppConfig{
		Server:     "https://botsin.space",
		ClientName: "kernelsprite",
		Scopes:     "read write follow",
		Website:    "https://github.com/oct2pus/kernelsprite",
	})
	if err != nil {
		log.Fatal(err)
	}
	c := mastodon.NewClient(&mastodon.Config{
		Server:       "https://botsin.space",
		ClientID:     app.ClientID,
		ClientSecret: app.ClientSecret,
	})
	fmt.Printf("Authenticate: %v\nEnter authorization: ", app.AuthURI)
	code, err := buf.ReadString('\n')
	if err != nil {
		log.Fatal(err)
	}
	// there is probably a better way of doing this
	code = strings.Replace(code, "\n", "", -1)
	err = c.AuthenticateToken(context.Background(), code, app.RedirectURI)
	if err != nil {
		log.Fatal(err)
	}
	// open BoltDB
	db, err := bolt.Open("./ks.db", 0644, nil)
	if err != nil {
		log.Fatal(err)
	}
	db.Update(func(tx *bolt.Tx) error {
		_, err = tx.CreateBucketIfNotExists(posts)
		if err != nil {
			return err
		}
		_, err = tx.CreateBucketIfNotExists(followers)
		if err != nil {
			return err
		}

		return nil
	})

	defer db.Close()
	// loop for infinity
	sleep := time.Duration(10)
	for {
		fmt.Println("==============")
		checkHashtag(c, db)
		time.Sleep(sleep * time.Second)
		fmt.Println("==============")
		checkFollows(c, db)
		time.Sleep(sleep * time.Second)
	}
}

func checkHashtag(c *mastodon.Client, db *bolt.DB) {
	statuses, err := c.GetTimelineHashtag(context.Background(), "HomestuckReread2020", false, nil)
	if err != nil {
		log.Printf("Could not check hashtag: %v.\nMoving on...\n", err)
		return
	}
	var errorMessage string
	fmt.Printf("Found %v statuses\n", len(statuses))
	notStored := make([]*mastodon.Status, 0, len(statuses))
	if len(statuses) > 0 {
		err = db.View(func(tx *bolt.Tx) error {
			bucket := tx.Bucket(posts)
			for _, ele := range statuses {
				value := bucket.Get([]byte(ele.URL))
				if value == nil {
					notStored = append(notStored, ele)
					_, err := c.Reblog(context.Background(), ele.ID)
					if err != nil {
						errorMessage = "View statuses - could not reblog post."
						return err
					}
				}
			}
			return nil
		})
		if err != nil {
			logError(errorMessage, err)
		}
	}
	fmt.Printf("Storing %v new statuses.\n", len(notStored))
	if len(notStored) > 0 {
		err = db.Update(func(tx *bolt.Tx) error {
			bucket := tx.Bucket(posts)
			for _, ele := range notStored {
				err = bucket.Put([]byte(ele.URL), []byte(ele.ID))
				if err != nil {
					errorMessage = "update statuses - could not store status"
					return err
				}
			}
			return nil
		})
		if err != nil {
			logError(errorMessage, err)
		}
	}
}

func checkFollows(c *mastodon.Client, db *bolt.DB) {
	self, err := c.GetAccountCurrentUser(context.Background())
	if err != nil {
		log.Printf("Lost in the sauce: %v\nMoving on...\n", err)
		return
	}
	var errorMessage string
	// the way this is being checked feels like it could become a bottleneck if
	// there are somehow thousands of followers. Check this first before
	// looking for other performance optimizations.
	accounts, err := c.GetAccountFollowers(context.Background(), self.ID, nil)
	if err != nil {
		log.Printf("")
	}
	fmt.Printf("Found %v Followers.\n", len(accounts))
	notStored := make([]*mastodon.Account, 0, len(accounts))
	if len(accounts) > 0 {
		err = db.View(func(tx *bolt.Tx) error {
			bucket := tx.Bucket(followers)
			for _, ele := range accounts {
				value := bucket.Get([]byte(ele.URL))
				if value == nil {
					notStored = append(notStored, ele)
					_, err := c.AccountFollow(context.Background(), ele.ID)
					if err != nil {
						errorMessage = "View followers - could not follow follower"
						return err
					}
				}
			}
			return nil
		})
		if err != nil {
			logError(errorMessage, err)
		}
	}
	fmt.Printf("Storing %v new followers.\n", len(notStored))
	if len(notStored) > 0 {
		err = db.Update(func(tx *bolt.Tx) error {
			bucket := tx.Bucket(followers)
			if err != nil {
				errorMessage = "Update followers - could not create followers db."
				return err
			}
			for _, ele := range notStored {
				err = bucket.Put([]byte(ele.URL), []byte(ele.ID))
				if err != nil {
					errorMessage = "Update followers - could not store follower."
					return err
				}
			}
			return nil
		})
		if err != nil {
			logError(errorMessage, err)
		}
	}
}

func logError(message string, err error) {
	log.Printf(message+"\nerror: %v\nMoving on...\n", err)
}
