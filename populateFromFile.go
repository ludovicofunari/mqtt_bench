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

func populateFromFile(fileName string) (Users, []map[string]byte, map[int]string) {

	file, err := ioutil.ReadFile(fileName)
	if err != nil {
		fmt.Println("Error reading file: ", err)
	}

	var user Users
	err = json.Unmarshal(file, &user)
	if err != nil {
		fmt.Println(err)
	}

	nodeIDs := make(map[int]string)
	nodePort := "31708"
	nodeIDs[0] = "tcp://localhost:1883"
	nodeIDs[1] = "tcp://192.168.3.4:" + nodePort
	nodeIDs[2] = "tcp://192.168.3.5:" + nodePort

	var arraySubTopics []map[string]byte
	subTopics := make(map[string]byte)

	var str string

	for _, sub := range user.Subscribers {
		for _, top := range sub.TopicList {
			str = strconv.Itoa(top)
			subTopics[str] = 0
		}
		arraySubTopics = append(arraySubTopics, subTopics)
	}

	return user, arraySubTopics, nodeIDs
}
