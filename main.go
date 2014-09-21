package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"runtime/debug"
	"time"

	"code.google.com/p/go-uuid/uuid"
	"github.com/codegangsta/negroni"
	"github.com/dgrijalva/jwt-go"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/northerntrickle/backend/httputil"
)

const (
	tileWidth = 16
	mapHeight = 100
	mapWidth  = 100
)

type userAttributes struct {
	Health int `json:"health"` // can be from 0-6. each decrement is a hit.
}

type position struct {
	Direction  direction `json:"direction"`
	Dimensions rect      `json:"dimensions"`
}

type rect struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

func (r *rect) Intersects(o *rect) bool {
	if r.X < o.X+o.Width &&
		r.X+r.Width > o.X &&
		r.Y < o.Y+o.Height &&
		r.Y+r.Height > o.Y {
		return true
	}
	return false
}

type User struct {
	ID         string         `json:"id"`
	Username   string         `json:"username"`
	Password   string         `json:"password,omitempty"`
	Attributes userAttributes `json:"attributes"`
	Position   position       `json:"position"`
}

func NewUser(username, password string) *User {
	user := &User{
		ID:       uuid.NewUUID().String(),
		Username: username,
		Password: password,
		Attributes: userAttributes{
			Health: 6,
		},
		Position: position{
			Dimensions: rect{
				Width:  tileWidth,
				Height: tileWidth * 2,
			},
		},
	}
	db.Users[user.ID] = user
	return user
}

type Guild struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	OwnerID   string   `json:"owner_id"`
	MemberIDs []string `json:"member_ids"`
}

func NewGuild(name, ownerID string) *Guild {
	guild := &Guild{
		ID:        uuid.NewUUID().String(),
		Name:      name,
		OwnerID:   ownerID,
		MemberIDs: make([]string, 0),
	}
	db.Guilds[guild.ID] = guild
	return guild
}

const dbname = "db.json"

type database struct {
	Users  map[string]*User
	Guilds map[string]*Guild
}

func (d *database) saveOnInterval(interval time.Duration) {
	t := time.NewTicker(interval)

	for {
		select {
		case <-t.C:
			// TODO: Stream this to the file
			b, err := json.Marshal(&d)
			if err != nil {
				log.Println(err)
				break
			}

			if err := ioutil.WriteFile(dbname, b, 0644); err != nil {
				log.Println(err)
				break
			}
		}
	}
}

func (d *database) getUserByUsername(username string) (u *User, err error) {
	for _, v := range d.Users {
		if v.Username == username {
			return v, nil
		}
	}
	return nil, errors.New("not found")
}

func NewDatabase() *database {
	return &database{
		Users:  make(map[string]*User),
		Guilds: make(map[string]*Guild),
	}
}

var db = NewDatabase()

type JWT struct {
	Tok *jwt.Token
}

func (t *JWT) Valid() bool {
	return t.Tok.Valid
}

func (t *JWT) String() string {
	str, err := t.Tok.SignedString([]byte("foobar"))
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

func NewJWTFromSignedString(signed string) (*JWT, error) {
	token, err := jwt.Parse(signed, func(token *jwt.Token) (interface{}, error) {
		return []byte("foobar"), nil
	})
	if err != nil {
		return nil, err
	}
	return &JWT{token}, nil
}

type eventType int

const (
	playerMove eventType = iota
	playerAttack
	playerDamage
	chat
)

type event struct {
	Type   eventType   `json:"type"`
	UserID string      `json:"user_id"`
	Body   interface{} `json:"body"`
}

func (e *event) UnmarshalJSON(data []byte) error {
	var typeWrapper struct {
		Type eventType `json:"type"`
	}
	if err := json.Unmarshal(data, &typeWrapper); err != nil {
		return err
	}

	e.Type = typeWrapper.Type

	switch e.Type {
	case playerMove:
		var wrapper struct {
			Body moveEvent `json:"body"`
		}
		if err := json.Unmarshal(data, &wrapper); err != nil {
			return err
		}
		e.Body = wrapper.Body
	case playerAttack:
		e.Body = nil
	case chat:
		var wrapper struct {
			Body chatEvent `json:"body"`
		}
		if err := json.Unmarshal(data, &wrapper); err != nil {
			return err
		}
		e.Body = wrapper.Body
	}

	return nil
}

type direction int

const (
	north direction = iota
	east
	south
	west
)

type moveEvent struct {
	Direction direction `json:"direction"`
}

type chatEvent struct {
	Msg string `json:"msg"`
}

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 512
)

type conn struct {
	ws     *websocket.Conn
	send   chan []byte
	userID string
}

func (c *conn) readPump() {
	defer func() {
		h.unregister <- c
		c.ws.Close()
	}()
	c.ws.SetReadLimit(maxMessageSize)
	c.ws.SetReadDeadline(time.Now().Add(pongWait))
	c.ws.SetPongHandler(func(string) error {
		c.ws.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})
	for {
		var evt event
		err := c.ws.ReadJSON(&evt)
		if err != nil {
			break
		}

		evt.UserID = c.userID

		switch evt.Type {
		case playerMove:
			user := db.Users[c.userID]
			body := evt.Body.(moveEvent)

			inc := 1.0
			switch body.Direction {
			case north:
				if user.Position.Dimensions.Y >= 0 {
					user.Position.Dimensions.Y -= inc
				} else {
					user.Position.Dimensions.Y += inc
				}
			case east:
				if user.Position.Dimensions.X+user.Position.Dimensions.Width <
					mapHeight*tileWidth {
					user.Position.Dimensions.X += inc
				} else {
					user.Position.Dimensions.X -= inc
				}
			case south:
				if user.Position.Dimensions.Y+user.Position.Dimensions.Height <
					mapHeight*tileWidth {
					user.Position.Dimensions.Y += inc
				} else {
					user.Position.Dimensions.Y -= inc
				}
			case west:
				if user.Position.Dimensions.X > 0 {
					user.Position.Dimensions.X -= inc
				} else {
					user.Position.Dimensions.X += inc
				}
			}

			evt.Body = user.Position

			b, err := json.Marshal(&evt)
			if err != nil {
				break
			}

			h.broadcast <- b
		case playerAttack:
			userPtr := db.Users[c.userID]
			user := *userPtr
			for _, v := range db.Users {
				if user.Username != v.Username &&
					user.Position.Dimensions.Intersects(&v.Position.Dimensions) {

					userPtr.Attributes.Health--
					user := *userPtr
					if user.Attributes.Health == 0 {
						delete(db.Users, user.ID)
					}

					e := event{
						Type:   playerDamage,
						UserID: user.ID,
						Body:   user.Attributes,
					}
					b, err := json.Marshal(&e)
					if err != nil {
						break
					}

					h.broadcast <- b

					e = event{
						Type:   chat,
						UserID: user.ID,
						Body: chatEvent{
							Msg: fmt.Sprintf("%s is dominating %s!", user.Username,
								v.Username),
						},
					}
					b, err = json.Marshal(&e)
					if err != nil {
						break
					}

					h.broadcast <- b
				}
			}

		case chat:
			b, err := json.Marshal(&evt)
			if err != nil {
				break
			}

			h.broadcast <- b
		}
	}
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
	errNotFound = &httputil.HTTPError{http.StatusNotFound,
		errors.New("not found")}
)

type handler func(w http.ResponseWriter, r *http.Request) error

func (h handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if rv := recover(); rv != nil {
			err := errors.New("handler panic")
			logError(r, err, rv)
			handleAPIError(w, r, http.StatusInternalServerError, err, false)
		}
	}()
	var rb httputil.ResponseBuffer
	err := h(&rb, r)
	if err == nil {
		rb.WriteTo(w)
	} else if e, ok := err.(*httputil.HTTPError); ok {
		if e.Status >= 500 {
			logError(r, err, nil)
		}
		handleAPIError(w, r, e.Status, e.Err, true)
	} else {
		logError(r, err, nil)
		handleAPIError(w, r, http.StatusInternalServerError, err, false)
	}
}

func logError(req *http.Request, err error, rv interface{}) {
	if err != nil {
		var buf bytes.Buffer
		fmt.Fprintf(&buf, "Error serving %s: %v\n", req.URL, err)
		if rv != nil {
			fmt.Fprintln(&buf, rv)
			buf.Write(debug.Stack())
		}
		log.Println(buf.String())
	}
}

func handleAPIError(resp http.ResponseWriter, req *http.Request,
	status int, err error, showErrorMsg bool) {
	var data struct {
		Error struct {
			Status  int    `json:"status"`
			Message string `json:"message"`
		} `json:"error"`
	}
	data.Error.Status = status
	if showErrorMsg {
		data.Error.Message = err.Error()
	} else {
		data.Error.Message = http.StatusText(status)
	}
	resp.Header().Set("Content-Type", "application/json; charset=utf-8")
	resp.WriteHeader(status)
	json.NewEncoder(resp).Encode(&data)
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
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}
)

func loadDBFromFile(filename string) error {
	file, err := os.Open(dbname)
	if err != nil && !os.IsNotExist(err) {
		return err
	} else if err == nil {
		if err := json.NewDecoder(file).Decode(&db); err != nil {
			return err
		}
	}
	return nil
}

func main() {
	if err := loadDBFromFile(dbname); err != nil {
		log.Fatal(err)
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}

	go h.run()
	go db.saveOnInterval(time.Second * 5)

	n := negroni.New()

	r := mux.NewRouter()

	r.Handle("/", handler(serveRoot)).Methods("GET")
	r.Handle("/static/",
		http.StripPrefix("/static/",
			http.FileServer(http.Dir("./static/")))).Methods("GET")

	r.Handle("/sign_up", handler(createUser)).Methods("POST")
	r.Handle("/login", handler(createJWT)).Methods("POST")

	r.HandleFunc("/connect", serveWs).Methods("GET")

	n.Use(negroni.HandlerFunc(func(w http.ResponseWriter, r *http.Request,
		next http.HandlerFunc) {
		w.Header().Add("Access-Control-Allow-Origin", "*")
		w.Header().Add("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == "OPTIONS" {
			return
		}
		next(w, r)
	}))

	n.UseHandler(r)
	n.Run(":" + port)
}

func renderJSON(w http.ResponseWriter, v interface{}, status int) error {
	w.WriteHeader(status)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	return json.NewEncoder(w).Encode(&v)
}

func serveRoot(w http.ResponseWriter, r *http.Request) error {
	file, err := os.Open("./index.html")
	if err != nil {
		return err
	}
	io.Copy(w, file)
	return nil
}

// TODO: Return error if username is already taken instead of just overwriting
// the user
func createUser(w http.ResponseWriter, r *http.Request) error {
	defer r.Body.Close()
	username, password, err := decodeUsernameAndPassword(r.Body)
	if err != nil {
		return err
	}

	for _, v := range db.Users {
		if v.Username == username {
			return &httputil.HTTPError{httputil.StatusUnprocessableEntity,
				errors.New("username taken")}
		}
	}

	user := *NewUser(username, password)
	user.Password = ""
	return renderJSON(w, user, http.StatusCreated)
}

func createJWT(w http.ResponseWriter, r *http.Request) error {
	defer r.Body.Close()
	username, password, err := decodeUsernameAndPassword(r.Body)
	if err != nil {
		return err
	}

	user, err := db.getUserByUsername(username)
	if err != nil {
		return errNotFound
	}

	if user.Password != password {
		return &httputil.HTTPError{httputil.StatusUnprocessableEntity,
			errors.New("password does not match")}
	}

	jwt := NewJWT(user.ID)
	var resp struct {
		UserID string `json:"user_id"`
		Token  string `json:"token"`
	}
	resp.UserID = user.ID
	resp.Token = jwt.String()
	return renderJSON(w, resp, http.StatusCreated)
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
	tokStr := r.URL.Query().Get("auth")
	if tokStr == "" {
		log.Println(&httputil.HTTPError{httputil.StatusUnprocessableEntity,
			errors.New("auth param is required")})
		return
	}

	tok, err := NewJWTFromSignedString(tokStr)
	if err != nil {
		log.Println(err)
		return
	}

	if !tok.Valid() {
		log.Println(&httputil.HTTPError{httputil.StatusUnprocessableEntity,
			errors.New("bad authentication")})
		return
	}

	user, ok := db.Users[tok.Tok.Claims["id"].(string)]
	if !ok {
		log.Println(&httputil.HTTPError{http.StatusNotFound,
			errors.New("user for token does not exist")})
		return
	}

	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}

	c := &conn{send: make(chan []byte, 256), ws: ws, userID: user.ID}
	h.register <- c
	go c.writePump()
	c.readPump()
}
