package main

import (
	"bufio"
	"encoding/xml"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"strings"
)

type IrcChannelMsg struct {
	channel string
	message string
}

func (msg IrcChannelMsg) getMessage() []byte {
	return []byte("PRIVMSG " + msg.channel + " :" + msg.message)
}

type Ircmsg struct {
	message string
}

func (msg Ircmsg) getMessage() []byte {
	return []byte(msg.message)
}

type Ircmessage interface {
	getMessage() []byte
}

func gettemp() string {

	type Weather struct {
		Temp string `xml:"value>value"`
	}

	resp, err := http.Get("http://opendata-download-metobs.smhi.se/api/version/latest/parameter/1/station/74460/period/latest-hour/data.xml")
	if err != nil {
		log.Print(err)
	}

	defer resp.Body.Close()
	xmldata, err := ioutil.ReadAll(resp.Body)

	var v Weather
	xml.Unmarshal([]byte(xmldata), &v)
	return v.Temp

}

func ircsender(conn io.Writer, c chan Ircmessage) {
	for {
		conn.Write((<-c).getMessage())
		conn.Write([]byte("\r\n"))
	}
}

func main() {
	c := make(chan Ircmessage)

	channelPtr := flag.String("channel", "", "Channel name")
	nick := flag.String("nick", "", "Nickname")
	server := flag.String("server", "", "IRC Server, host:port")

	flag.Parse()

	channel := "#" + *channelPtr

	conn, err := net.Dial("tcp", *server)
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Fprintln(conn, "USER", *nick, "", *nick, "", *nick, ":", *nick)
	fmt.Fprintln(conn, "NICK", *nick)
	fmt.Fprintln(conn, "JOIN", channel)

	reader := bufio.NewReader(conn)
	go ircsender(conn, c)
	for {

		line, _ := reader.ReadString('\n')
		data := strings.Split(line, ":")

		cmd := strings.TrimSpace(data[0])
		val := ""
		arg := ""

		if len(data) > 1 {

			val = strings.TrimSpace(data[1])

		}
		if len(data) > 2 {
			arg = strings.TrimSpace(data[2])
		}
		log.Print(cmd, val, arg)
		if cmd == "PING" {
			c <- Ircmsg{"PONG " + val}
		}

		if arg == "!mupp" {
			c <- IrcChannelMsg{channel, "Muppelimupp!"}
		}

		if arg == "!mupp temp" {
			c <- IrcChannelMsg{channel, "I Jönköping (på flygplatsen) är det " + gettemp() + " grader."}
		}
	}
}
