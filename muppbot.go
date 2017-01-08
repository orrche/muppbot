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

func main() {

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
			fmt.Fprintln(conn, "PONG", val)
		}

		if arg == "!mupp" {
			fmt.Fprintln(conn, "PRIVMSG", channel, ":Muppelimupp!")
		}

		if arg == "!mupp temp" {
			fmt.Fprintln(conn, "PRIVMSG", channel, ":I Jönköping (på flygplatsen) är det", gettemp(), "grader.")
		}
	}
}
