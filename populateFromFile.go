package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
)

type Publishers struct {
	Publishers []Publisher `json:"publisher"`
}

type Users struct {
	Publishers []Publisher `json:"publisher"`
	Subscribers []Subscriber `json:"subscriber"`
}

type Publisher struct {
	PubID 		float64 `json:"pub_id"`
	NodeID 		int `json:"node_id"`
	TopicList 	[]int `json:"topic_list"`
}

type Subscriber struct {
	SubID 		float64 `json:"sub_id"`
	NodeID 		int `json:"node_id"`
	TopicList 	[]int `json:"topic_list"`
}

func populateFromFile(fileName string) Users {
	file, err := ioutil.ReadFile(fileName)
	if err != nil {
		fmt.Println("Error reading file: ", err)
	}

	var user Users
	err = json.Unmarshal(file, &user)
	if err != nil {
		fmt.Println(err)
	}
	return user
}