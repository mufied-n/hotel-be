package handler

import (
	"net/http"

	"github.com/example/hotel-booking/internal/api/http/middleware"
	"github.com/example/hotel-booking/internal/catalog"
	"github.com/gin-gonic/gin"
)

// GetCatalogRooms mengembalikan daftar varian kamar yang tersedia di katalog (GET /api/v1/catalog/rooms).
func GetCatalogRooms(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		variants, err := d.CatalogStore.ListVariants(c.Request.Context())
		if err != nil {
			writeDomainError(c, err, nil, listCatalogFallback)
			return
		}
		middleware.WriteJSON(c, http.StatusOK, map[string]any{
			"total": len(variants),
			"rooms": variants,
		})
	}
}

// GetCatalogRoom mengembalikan detail varian kamar berdasarkan ID (GET /api/v1/catalog/rooms/{id}).
func GetCatalogRoom(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		v, err := d.CatalogStore.GetVariant(c.Request.Context(), id)
		if err != nil {
			writeDomainError(c, err, getCatalogRoomErrors, getCatalogRoomFallback)
			return
		}
		middleware.WriteJSON(c, http.StatusOK, v)
	}
}

// CreateCatalogRoom membuat varian kamar baru di katalog (POST /api/v1/catalog/rooms).
func CreateCatalogRoom(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		var v catalog.RoomVariant
		if !middleware.DecodeJSON(c, &v, "INVALID_ROOM_PAYLOAD", "body JSON tidak valid") {
			return
		}
		created, err := d.CatalogStore.CreateVariant(c.Request.Context(), v)
		if err != nil {
			writeDomainError(c, err, createCatalogErrors, createCatalogFallback)
			return
		}
		if d.RateEngine != nil {
			d.RateEngine.SetBaseRate(created.ID, created.BasePriceMinor)
		}
		middleware.WriteJSON(c, http.StatusCreated, created)
	}
}

// UpdateCatalogRoom memperbarui spesifikasi varian kamar di katalog (PUT /api/v1/catalog/rooms/{id}).
func UpdateCatalogRoom(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		var v catalog.RoomVariant
		if !middleware.DecodeJSON(c, &v, "INVALID_ROOM_PAYLOAD", "body JSON tidak valid") {
			return
		}
		updated, err := d.CatalogStore.UpdateVariant(c.Request.Context(), id, v)
		if err != nil {
			writeDomainError(c, err, updateCatalogErrors, updateCatalogFallback)
			return
		}
		if d.RateEngine != nil {
			d.RateEngine.SetBaseRate(updated.ID, updated.BasePriceMinor)
		}
		middleware.WriteJSON(c, http.StatusOK, updated)
	}
}

// DeleteCatalogRoom menghapus varian kamar dari katalog (DELETE /api/v1/catalog/rooms/{id}).
func DeleteCatalogRoom(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		err := d.CatalogStore.DeleteVariant(c.Request.Context(), id)
		if err != nil {
			writeDomainError(c, err, deleteCatalogErrors, deleteCatalogFallback)
			return
		}
		middleware.WriteJSON(c, http.StatusOK, map[string]string{"status": "deleted", "id": id})
	}
}
