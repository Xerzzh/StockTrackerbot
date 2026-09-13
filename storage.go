package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
)

// configDir es el directorio base donde se persisten los ficheros de
// configuración. Se puede sobrescribir desde los tests.
var configDir = "config"

// productsPath devuelve la ruta al JSON de productos del usuario.
func productsPath(chatID int64) string {
	return filepath.Join(configDir, fmt.Sprintf("products_%d.json", chatID))
}

// saveProducts persiste los productos del usuario. El llamador debe sostener
// config.Mutex o garantizar que nadie muta el slice.
func saveProducts(config *UserConfig) {
	saveProductsSnapshot(config.ChatID, config.Products)
}

// saveProductsSnapshot escribe a disco una copia ya extraída del slice.
func saveProductsSnapshot(chatID int64, products []Product) {
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		log.Printf("❌ Error creando directorio de config: %v", err)
		return
	}

	storage := UserStorage{Products: products}
	data, err := json.MarshalIndent(storage, "", "  ")
	if err != nil {
		log.Printf("❌ Error serializando productos: %v", err)
		return
	}
	if err := os.WriteFile(productsPath(chatID), data, 0o644); err != nil {
		log.Printf("❌ Error escribiendo productos: %v", err)
	}
}

// loadProducts carga los productos del usuario desde disco (si existen).
func loadProducts(config *UserConfig) {
	data, err := os.ReadFile(productsPath(config.ChatID))
	if err != nil {
		return
	}
	var storage UserStorage
	if err := json.Unmarshal(data, &storage); err != nil {
		log.Printf("❌ Error parseando productos del usuario %d: %v", config.ChatID, err)
		return
	}
	config.Products = storage.Products
}
