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
			log.Printf("abnormal closed:%v", err)
		}
		return
	}

	Config.Data = func(c evnet.Conn, in []byte, out *([]byte)) (action evnet.Action) {
		log.Printf("get msg:%s", string(in))
		length := len(in)
		leftLength := len(*out)

		if leftLength >= length {
			copy(*out, in)
			*out = (*out)[length:]
		} else {

		}
		return
	}

	Evnet.Serve(Config, "tcp://127.0.0.1:3777")
}
