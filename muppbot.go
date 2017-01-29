package main

import (
	"bufio"
	"bytes"
	"encoding/xml"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gliderlabs/ssh"
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

type User struct {
	key  []byte
	user string
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

var users []User

func findUser(key []byte) User {
	for _, user := range users {
		if bytes.Equal(key, user.key) {
			return user
		}
	}
	return User{}
}

func paniconerr(err error, msg string) {
	if err != nil {
		log.Panic(msg, err)
	}
}

func purgeFiles() {
	for {
		folders, _ := ioutil.ReadDir("ssh-data")
		for _, folder := range folders {
			fPath := path.Join("ssh-data", folder.Name())

			files, _ := ioutil.ReadDir(fPath)
			for _, file := range files {
				filePath := path.Join(fPath, file.Name())

				var st syscall.Stat_t
				if err := syscall.Stat(filePath, &st); err != nil {
					log.Fatal(err)
				}
				age := time.Now().Unix() - st.Mtim.Sec
				log.Printf("file %s age: %d\n", filePath, age)

				if age > 60 {
					os.RemoveAll(fPath)
				}

			}
		}
		time.Sleep(time.Minute)
	}
}

func sshserver(ircc chan Ircmessage, weburl string) {
	go purgeFiles()
	users = make([]User, 10, 10)

	files, _ := ioutil.ReadDir("keys")
	for _, f := range files {
		if f.IsDir() {
			keyFiles, _ := ioutil.ReadDir(path.Join("keys", f.Name()))
			for _, keyFile := range keyFiles {
				data, err := ioutil.ReadFile(path.Join("keys", f.Name(), keyFile.Name()))
				paniconerr(err, "Problem reading file")
				key, _, _, _, err := ssh.ParseAuthorizedKey(data)
				paniconerr(err, "Problem parsing keyfile")

				users = append(users, User{key.Marshal(), f.Name()})
			}
		}
	}

	ssh.Handle(func(s ssh.Session) {
		user := findUser(s.PublicKey().Marshal())

		reader := bufio.NewReader(s)

		if s.Command()[0] == "scp" {
			io.WriteString(s, "\000")
			scpLine, _ := reader.ReadString('\n')
			scpData := strings.Split(scpLine, " ")
			filename := strings.TrimSpace(scpData[2])
			size64, err := strconv.ParseInt(scpData[1], 10, 64)
			size := int(size64)

			tempdir, err := ioutil.TempDir("ssh-data", user.user)

			if err == nil {
				msg := fmt.Sprintf("%s is pushing filename: %s (%d)", user.user, filename, size)
				log.Print(msg)
				url := fmt.Sprintf(weburl + strings.SplitN(tempdir, "/", 2)[1] + "/" + filename)
				log.Print(url)
				ircc <- IrcChannelMsg{"#" + s.Command()[2], msg}

				io.WriteString(s, "\000")
			} else {
				io.WriteString(s, "\001")
			}
			f, err := os.Create(tempdir + "/" + filename)
			defer f.Close()

			for i := 0; i < size; {
				toread := 1024
				if (size - i) < toread {
					toread = size - i
				}
				data := make([]byte, toread, toread)
				length, err := io.ReadAtLeast(reader, data, toread)
				i += length

				if err != nil {
					io.WriteString(s, "\001")
					log.Panic(err)
				}

				f.Write(data[:length])
			}
			io.WriteString(s, "\000")
		} else {
			log.Print(s.Command()[0])
		}
	})

	log.Fatal(ssh.ListenAndServe(":2222", nil,
		ssh.HostKeyFile("ssh/gogs.rsa"),
		ssh.PublicKeyAuth(func(user string, key ssh.PublicKey) bool {
			return findUser(key.Marshal()).user != ""
		}),
	))
}

func ircsender(conn io.Writer, c chan Ircmessage) {
	for {
		conn.Write((<-c).getMessage())
		conn.Write([]byte("\r\n"))
	}
}

func main() {
	c := make(chan Ircmessage)

	fs := http.FileServer(http.Dir("ssh-data"))
	http.Handle("/", fs)

	go func() { http.ListenAndServe(":8080", nil) }()

	channelPtr := flag.String("channel", "", "Channel name")
	nick := flag.String("nick", "", "Nickname")
	pass := flag.String("pass", "", "Password")
	server := flag.String("server", "", "IRC Server, host:port")
	weburl := flag.String("url", "", "WebUrl, http://localhost:8000")

	go sshserver(c, *weburl)
	flag.Parse()

	channel := "#" + *channelPtr

	conn, err := net.Dial("tcp", *server)
	if err != nil {
		fmt.Println(err)
		return
	}

	if len(*pass) > 0 {
		fmt.Fprintln(conn, "PASS", *pass)
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
