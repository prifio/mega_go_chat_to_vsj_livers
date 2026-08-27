package toplevel

import (
	"vcmsg/room"
)

type HashedPass struct {
	Hash string
}

type User struct {
	Rooms    map[string]bool
	PassHash HashedPass
}

type Manager struct {
	Rooms map[string]*room.Manager
	Users map[string]*User
}

func PasswordToHash(password string) HashedPass {
	return HashedPass{
		Hash: password, // TODO!
	}
}

func CheckPass(hp HashedPass, password string) bool {
	return hp.Hash == password // TODO!
}

func InitTestManager() *Manager {
	mgr := Manager{
		Rooms: map[string]*room.Manager{},
		Users: map[string]*User{},
	}

	names := []string{"ship", "land"}
	for _, name := range names {
		mgr.Rooms[name] = room.NewManager(name, 8)
		go mgr.Rooms[name].Start()
	}
	users := []string{"prifio_k", "chuzezemets", "late_man"}
	for _, uname := range users {
		mgr.Users[uname] = &User{
			Rooms:    map[string]bool{},
			PassHash: PasswordToHash("12345678"),
		}
	}
	// TODO: make AddUser blockable
	mgr.Rooms[names[0]].AddUser(users[0])
	mgr.Users[users[0]].Rooms[names[0]] = true

	mgr.Rooms[names[0]].AddUser(users[1])
	mgr.Users[users[1]].Rooms[names[0]] = true

	mgr.Rooms[names[1]].AddUser(users[0])
	mgr.Users[users[0]].Rooms[names[1]] = true

	mgr.Rooms[names[1]].AddUser(users[2])
	mgr.Users[users[2]].Rooms[names[1]] = true
	return &mgr
}

func (tlm *Manager) LaunchRooms() {
	for _, r := range tlm.Rooms {
		go r.Start()
	}
}
