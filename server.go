package main

import (
    "net"
    "fmt"
    "log"
    "encoding/binary"
    "encoding/json"
    "math/rand"
    "sync"
)

type User struct {
    name string
    id int
    conn net.Conn
    pistol_state bool
    searches_game bool
    x int
    y int
}

type UserMeta struct {
    Name string
    Id int
}

var WIDTH = 800
var HEIGHT = 600
var RADIUS = 20

type Users struct {
    inner map[int]*User
    sync.Mutex
}

type Games struct {
    inner map[int]int
    sync.Mutex
}

var users Users
var games Games

func randomGameSearcher(searchRandomGame chan struct{}) {
    for {
        <-searchRandomGame

        var users_to_search []*User

        users.Lock()
        for _, user := range users.inner {
            if user.searches_game {
                users_to_search = append(users_to_search, user)
            }
        }
        users.Unlock()

        rand.Intn(len(users_to_search))
    }
}

func main() {
    srv, err := net.Listen("tcp", ":4242")

    if err != nil {
        log.Panic(err)
    }

    fmt.Println("Server is running on port 4242")

    users.inner = make(map[int]*User)
    games.inner = make(map[int]int)

    searchRandomGame := make(chan struct{})
    go randomGameSearcher(searchRandomGame)

    for {
        conn, err := srv.Accept();

        if err != nil {
            log.Print(err)
        }

        go func() {
            buf := make([]byte, 30)
            
            bytesRead, err := conn.Read(buf)

            if err != nil {
                log.Print(err)
                return
            }

            user := User{
                name: string(buf[:bytesRead]),
                id: len(users.inner),
                conn: conn,
            }

            users.Lock()
            users.inner[user.id] = &user
            users.Unlock()

            for {
                buf = make([]byte, 50)

                _, err := conn.Read(buf)

                if err != nil {
                    log.Print(err)
                    return
                }

                if buf[0] == 0 {
                    // Request of listing all users

                    var users_list []UserMeta

                    users.Lock()
                    for _, curr_user := range users.inner {
                        if _, contains := games.inner[user.id]; !contains && user.id != curr_user.id {
                            users_list = append(
                                users_list,
                                UserMeta{ Name: curr_user.name, Id: curr_user.id },
                            )
                        }
                    }
                    users.Unlock()

                    buf, err = json.Marshal(users_list)

                    if err != nil {
                        log.Print(err)
                        return
                    }

                    conn.Write(buf)
                } else if buf[0] == 1 {
                    // User chose another user to play with

                    another_id := int(binary.LittleEndian.Uint32(buf[1:5]))

                    users.Lock()
                    games.Lock()
                    if _, contains := games.inner[another_id]; !contains {
                        another_user := users.inner[another_id]

                        fmt.Println("INV to", another_user.name)

                        games.inner[user.id] = another_id
                        games.inner[another_id] = user.id

                        another_user.conn.Write(append([]byte{ 0 }, []byte(user.name)...))
                    }
                    users.Unlock()
                    games.Unlock()
                } else if buf[0] == 2 {
                    // User pressed Search Game

                    user.searches_game = true

                    searchRandomGame <- struct{}{}
                } else if buf[0] == 3 {
                    // User accepted invitation

                    users.Lock()
                    games.Lock()
                    another_user := users.inner[games.inner[user.id]]
                    users.Unlock()
                    games.Unlock()

                    x1, y1 := rand.Intn(WIDTH - RADIUS), rand.Intn(HEIGHT - RADIUS)
                    user.x = x1 + RADIUS / 2
                    user.y = y1 + RADIUS / 2

                    x2, y2 := rand.Intn(WIDTH - RADIUS), rand.Intn(HEIGHT - RADIUS)
                    another_user.x = x2 + RADIUS / 2
                    another_user.y = y2 + RADIUS / 2

                    fmt.Printf("User %v coordinates: (%v, %v)\n", user.name, x1, y1)
                    fmt.Printf("User %v coordinates: (%v, %v)\n", another_user.name, x2, y2)

                    buf := make([]byte, 9)
                    buf[0] = 1
                    binary.LittleEndian.PutUint32(buf[1:], uint32(x2))
                    binary.LittleEndian.PutUint32(buf[5:], uint32(y2))
                    conn.Write(buf)

                    buf = make([]byte, 9)
                    buf[0] = 1
                    binary.LittleEndian.PutUint32(buf[1:], uint32(x1))
                    binary.LittleEndian.PutUint32(buf[5:], uint32(y1))
                    another_user.conn.Write(buf)
                } else if buf[0] == 4 {
                    // User pressed pistol button

                    user.pistol_state = true

                    users.Lock()
                    games.Lock()
                    users.inner[games.inner[user.id]].conn.Write([]byte{ 2 })
                    users.Unlock()
                    games.Unlock()
                } else if buf[0] == 5 {
                    // User shoots

                    users.Lock()
                    games.Lock()
                    if user.pistol_state {
                        // Get coordinates, check, send messages to 2 users

                        another_user := users.inner[games.inner[user.id]]

                        shot_x := int(binary.LittleEndian.Uint32(buf[1:]))
                        shot_y := int(binary.LittleEndian.Uint32(buf[5:]))

                        enemy_x := another_user.x
                        enemy_y := another_user.y

                        if (shot_x - enemy_x) * (shot_x - enemy_x) + (shot_y - enemy_y) * (shot_y - enemy_y) <= RADIUS * RADIUS {
                            another_user.conn.Write([]byte{ 3 })
                            conn.Write([]byte{ 4 })

                            delete(games.inner, user.id)
                            delete(games.inner, another_user.id)
                        }
                    }
                    users.Unlock()
                    games.Unlock()
                } else if buf[0] == 6 {
                    // User refuses to invitation

                    users.Lock()
                    games.Lock()

                    another_user := users.inner[games.inner[user.id]]

                    another_user.conn.Write([]byte{ 5 })

                    delete(games.inner, user.id)
                    delete(games.inner, another_user.id)

                    users.Unlock()
                    games.Unlock()
                } else if buf[0] == 7 {
                    // User stops searching game

                    user.searches_game = false
                }
            }
        }()
    }
}
