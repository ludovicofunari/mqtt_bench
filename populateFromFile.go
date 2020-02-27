package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strconv"
)

type Users struct {
	Publishers  []Publisher  `json:"publisher"`
	Subscribers []Subscriber `json:"subscriber"`
}

type Publisher struct {
	PubID     float64 `json:"pub_id"`
	NodeID    int     `json:"node_id"`
	TopicList []int   `json:"topic_list"`
}

type Subscriber struct {
	SubID     float64 `json:"sub_id"`
	NodeID    int     `json:"node_id"`
	TopicList []int   `json:"topic_list"`
}

func populateFromFile(fileName string) (Users, []map[string]byte) {
	file, err := ioutil.ReadFile(fileName)
	if err != nil {
		fmt.Println("Error reading file: ", err)
	}

	var user Users
	err = json.Unmarshal(file, &user)
	if err != nil {
		fmt.Println(err)
	}

	var arraySubTopics []map[string]byte
	subTopics := make(map[string]byte)

	var str string

	qos := byte(0)

	for _, sub := range user.Subscribers {
		for _, top := range sub.TopicList {
			str = strconv.Itoa(top)
			subTopics[str] = qos
		}
		arraySubTopics = append(arraySubTopics, subTopics)
	}

	return user, arraySubTopics
}
