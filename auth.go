package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

var (
	adminChatID     int64
	authorizedUsers = make(map[int64]bool)
	authMutex       sync.RWMutex
)

// authorizedUsersPath devuelve la ruta del fichero de autorizados.
func authorizedUsersPath() string {
	return filepath.Join(configDir, "authorized_users.txt")
}

// isAuthorized comprueba si un chatID tiene permiso para usar el bot. Si no
// hay admin configurado, el bot es de acceso libre.
func isAuthorized(chatID int64) bool {
	if adminChatID == 0 || chatID == adminChatID {
		return true
	}
	authMutex.RLock()
	defer authMutex.RUnlock()
	return authorizedUsers[chatID]
}

// loadAuthorizedUsers carga los IDs autorizados desde disco.
func loadAuthorizedUsers() {
	file, err := os.Open(authorizedUsersPath())
	if err != nil {
		return
	}
	defer file.Close()

	authMutex.Lock()
	defer authMutex.Unlock()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if id, err := strconv.ParseInt(line, 10, 64); err == nil {
			authorizedUsers[id] = true
		}
	}
	if err := scanner.Err(); err != nil {
		log.Printf("⚠️ Error leyendo authorized_users: %v", err)
	}
}

// saveAuthorizedUsers persiste el mapa actual en disco.
func saveAuthorizedUsers() {
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		log.Printf("❌ Error creando directorio de config: %v", err)
		return
	}

	authMutex.RLock()
	ids := make([]int64, 0, len(authorizedUsers))
	for id := range authorizedUsers {
		ids = append(ids, id)
	}
	authMutex.RUnlock()

	file, err := os.Create(authorizedUsersPath())
	if err != nil {
		log.Printf("❌ Error al guardar usuarios autorizados: %v", err)
		return
	}
	defer file.Close()

	w := bufio.NewWriter(file)
	for _, id := range ids {
		if _, err := fmt.Fprintf(w, "%d\n", id); err != nil {
			log.Printf("❌ Error escribiendo usuario %d: %v", id, err)
			return
		}
	}
	if err := w.Flush(); err != nil {
		log.Printf("❌ Error flush authorized_users: %v", err)
	}
}

// authorizeUser añade un usuario autorizado y lo persiste.
func authorizeUser(chatID int64) {
	authMutex.Lock()
	authorizedUsers[chatID] = true
	authMutex.Unlock()
	saveAuthorizedUsers()
}

// revokeUser revoca un usuario y lo persiste.
func revokeUser(chatID int64) {
	authMutex.Lock()
	delete(authorizedUsers, chatID)
	authMutex.Unlock()
	saveAuthorizedUsers()
}

// getAuthorizedUsersList devuelve la lista formateada para mostrar.
func getAuthorizedUsersList() string {
	authMutex.RLock()
	defer authMutex.RUnlock()

	if len(authorizedUsers) == 0 {
		return "📋 No hay usuarios autorizados."
	}

	var sb strings.Builder
	sb.WriteString("📋 *Usuarios autorizados:*\n")
	for id := range authorizedUsers {
		fmt.Fprintf(&sb, "- `%d`\n", id)
	}
	return sb.String()
}

// getUnauthorizedAdminMessage genera el mensaje que se envía al admin cuando
// alguien no autorizado intenta usar el bot.
func getUnauthorizedAdminMessage(chatID int64) string {
	return fmt.Sprintf("🚨 *Acceso no autorizado*\nID del usuario: `%d`", chatID)
}
