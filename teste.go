package main

import (
	"evgw/evnet"
	"log"
)

func main() {
	Evnet := evnet.NewEvnet()
	Config := evnet.NewConfig()

	Config.Serving = func() (action evnet.Action) {
		log.Println("server start")
		return
	}

	Config.Opened = func(c evnet.Conn, out []byte) (action evnet.Action) {
		log.Println("new client")
		return
	}

	Config.Closed = func(c evnet.Conn, err error) (action evnet.Action) {
		if err == nil {
			log.Printf("normal closed")
		} else {
			log.Printf("close:%v", err)
		}
		return
	}

	Evnet.Serve(Config, "tcp://127.0.0.1:3777")
}
