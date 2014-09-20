package main

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"code.google.com/p/go-uuid/uuid"

	"github.com/dgrijalva/jwt-go"
	"github.com/gorilla/websocket"
)

type database struct {
	users map[string]*User
}

func (d *database) getUserByUsername(username string) (u *User, err error) {
	for _, v := range d.users {
		if v.Username == username {
			return v, nil
		}
	}
	return nil, errors.New("not found")
}

func NewDatabase() *database {
	return &database{
		users: make(map[string]*User),
	}
}

var db = NewDatabase()

type User struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Password string `json:"-"`
}

type JWT struct {
	tok *jwt.Token
}

func (t *JWT) String() string {
	str, err := t.tok.SignedString([]byte("foobar"))
	if err != nil {
		panic(err)
	}
	return str
}

func NewJWT(userID string) *JWT {
	tok := jwt.New(jwt.GetSigningMethod("HS256"))
	tok.Claims["id"] = userID
	tok.Claims["exp"] = time.Now().Add(time.Hour * 72).Unix()
	return &JWT{tok}
}

func CreateUser(username, password string) *User {
	user := &User{
		ID:       uuid.NewUUID().String(),
		Username: username,
		Password: password,
	}
	db.users[user.ID] = user
	return user
}

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 512
)

type conn struct {
	ws   *websocket.Conn
	send chan []byte
}

func (c *conn) write(mt int, payload []byte) error {
	c.ws.SetWriteDeadline(time.Now().Add(writeWait))
	return c.ws.WriteMessage(mt, payload)
}
func (c *conn) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.ws.Close()
	}()
	for {
		select {
		case msg, ok := <-c.send:
			if !ok {
				c.ws.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.ws.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ticker.C:
			if err := c.write(websocket.PingMessage, []byte{}); err != nil {
				return
			}
		}
	}
}

type hub struct {
	conns      map[*conn]bool
	broadcast  chan []byte
	register   chan *conn
	unregister chan *conn
}

func (h *hub) run() {
	for {
		select {
		case c := <-h.register:
			h.conns[c] = true
		case c := <-h.unregister:
			if _, ok := h.conns[c]; ok {
				delete(h.conns, c)
				close(c.send)
			}
		case m := <-h.broadcast:
			for c := range h.conns {
				select {
				case c.send <- m:
				default:
					close(c.send)
					delete(h.conns, c)
				}
			}
		}
	}
}

var (
	h = &hub{
		conns:      make(map[*conn]bool),
		broadcast:  make(chan []byte),
		register:   make(chan *conn),
		unregister: make(chan *conn),
	}

	upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}
)

func httpLog(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		log.Printf("%s %s %s", r.RemoteAddr, r.Method, r.URL)
		handler.ServeHTTP(w, r)
		log.Printf("Completed in %s", time.Now().Sub(start).String())
	})
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}

	http.HandleFunc("/sign_up", createUser)
	http.HandleFunc("/login", createJWT)
	http.HandleFunc("/connect", serveWs)
	log.Fatal(http.ListenAndServe(":"+port, httpLog(http.DefaultServeMux)))
}

// TODO: Return error if username is already taken instead of just overwriting
// the user
func createUser(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	username, password, err := decodeUsernameAndPassword(r.Body)
	if err != nil {
		panic(err)
	}

	user := CreateUser(username, password)
	if err := json.NewEncoder(w).Encode(&user); err != nil {
		panic(err)
	}
}

func createJWT(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	username, password, err := decodeUsernameAndPassword(r.Body)
	if err != nil {
		panic(err)
	}

	user, err := db.getUserByUsername(username)
	if err != nil {
		panic(err)
	}

	if user.Password != password {
		w.Write([]byte("password does not match"))
		return
	}

	jwt := NewJWT(user.ID)
	var resp struct {
		Token string `json:"token"`
	}
	resp.Token = jwt.String()
	if err := json.NewEncoder(w).Encode(&resp); err != nil {
		panic(err)
	}
}

func decodeUsernameAndPassword(r io.Reader) (username, password string,
	err error) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r).Decode(&req); err != nil {
		return "", "", err
	}

	return req.Username, req.Password, nil
}

func serveWs(w http.ResponseWriter, r *http.Request) {
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}

	c := &conn{send: make(chan []byte, 256), ws: ws}
	h.register <- c
	c.writePump()
}
