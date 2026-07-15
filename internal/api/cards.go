package api

import (
	"errors"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/davidlc1229/omnishelf/internal/cards"
	"github.com/davidlc1229/omnishelf/internal/models"
)

// Machine error codes for the card endpoints.
const (
	CodeCardNotFound    = "card_not_found"
	CodeNoTextDetected  = "no_text_detected"
	CodeUnsupportedCard = "unsupported_card"
)

// maxCardImageBytes caps an uploaded card photo so a huge file cannot exhaust
// disk or memory. Phone photos run a few MB; 10 MiB is generous.
const maxCardImageBytes = 10 << 20

// cardsHandler serves the card scan/add endpoints.
type cardsHandler struct {
	svc *cards.Service
}

// RegisterCardRoutes attaches the card endpoints to the JWT-protected /api
// group returned by RegisterRoutes.
func RegisterCardRoutes(grp *gin.RouterGroup, svc *cards.Service) {
	h := &cardsHandler{svc: svc}
	grp.POST("/cards/scan", h.scan)
	grp.POST("/cards/add", h.add)
}

// cardScanResponse is the JSON shape of a successful scan: the identified
// card, ready to be posted back to /cards/add. coverPath is the cached
// artwork's path relative to the images root (serve it as /images/<coverPath>,
// like every other module); "" means no artwork.
type cardScanResponse struct {
	Game       string  `json:"game"`
	ExternalID string  `json:"externalId"`
	Name       string  `json:"name"`
	CardType   string  `json:"cardType"`
	Race       string  `json:"race"`
	Artist     string  `json:"artist"`
	SetCode    string  `json:"setCode"`
	SetName    string  `json:"setName"`
	Price      float64 `json:"price"`
	CoverPath  string  `json:"coverPath"`
}

// cardAddRequest is the /cards/add body: the scan result the client is
// shelving (coverPath included, so the artwork the scan cached survives the
// round-trip), plus an optional status (defaults to OWNED).
type cardAddRequest struct {
	Game       string  `json:"game"`
	ExternalID string  `json:"externalId"`
	Name       string  `json:"name"`
	CardType   string  `json:"cardType"`
	Race       string  `json:"race"`
	Artist     string  `json:"artist"`
	SetCode    string  `json:"setCode"`
	SetName    string  `json:"setName"`
	Price      float64 `json:"price"`
	CoverPath  string  `json:"coverPath"`
	Status     string  `json:"status"`
}

// cardResponse is the JSON shape of a Card payload.
type cardResponse struct {
	ID         uint    `json:"id"`
	ExternalID string  `json:"externalId"`
	Game       string  `json:"game"`
	Name       string  `json:"name"`
	CardType   string  `json:"cardType"`
	Race       string  `json:"race"`
	Artist     string  `json:"artist"`
	SetCode    string  `json:"setCode"`
	SetName    string  `json:"setName"`
	Price      float64 `json:"price"`
	CoverPath  string  `json:"coverPath"`
}

func toCardResponse(card *models.Card) cardResponse {
	return cardResponse{
		ID:         card.ID,
		ExternalID: card.ExternalID,
		Game:       card.Game,
		Name:       card.Name,
		CardType:   card.CardType,
		Race:       card.Race,
		Artist:     card.Artist,
		SetCode:    card.SetCode,
		SetName:    card.SetName,
		Price:      card.Price,
		CoverPath:  card.CoverPath,
	}
}

// scan handles POST /api/cards/scan — a card photo sent as multipart form
// field "card_image", pushed through the OCR identification waterfall.
func (h *cardsHandler) scan(c *gin.Context) {
	// Cap the request body before touching it so an oversized upload is
	// rejected without being buffered to disk.
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxCardImageBytes)
	fileHeader, err := c.FormFile("card_image")
	if err != nil {
		Error(c, http.StatusBadRequest, CodeInvalidRequest,
			"send the card photo as multipart form field \"card_image\" (max 10 MiB)")
		return
	}
	file, err := fileHeader.Open()
	if err != nil {
		Error(c, http.StatusBadRequest, CodeInvalidRequest, "could not read the uploaded file")
		return
	}
	defer func() { _ = file.Close() }()
	imageBytes, err := io.ReadAll(file)
	if err != nil || len(imageBytes) == 0 {
		Error(c, http.StatusBadRequest, CodeInvalidRequest, "could not read the uploaded file")
		return
	}

	res, err := h.svc.Scan(c.Request.Context(), imageBytes)
	var notFound *cards.NotFoundError
	switch {
	case errors.Is(err, cards.ErrNotConfigured):
		log.Print("card scan rejected: Vision OCR is not configured — set GOOGLE_APPLICATION_CREDENTIALS to a GCP service-account JSON file path")
		Error(c, http.StatusServiceUnavailable, CodeUpstreamError, "card scanning is not configured")
	case errors.Is(err, cards.ErrNoText):
		Error(c, http.StatusNotFound, CodeNoTextDetected, "No text detected on image")
	case errors.Is(err, cards.ErrUnsupportedCard):
		Error(c, http.StatusUnprocessableEntity, CodeUnsupportedCard, "Unsupported card type or unreadable set code")
	case errors.As(err, &notFound):
		// Echo what the waterfall classified so the UI can show it.
		c.JSON(http.StatusNotFound, gin.H{
			"error":   CodeCardNotFound,
			"message": "no catalog entry found for this card",
			"game":    notFound.Game,
			"setCode": notFound.SetCode,
		})
	case errors.Is(err, cards.ErrCardNotFound):
		Error(c, http.StatusNotFound, CodeCardNotFound, "no catalog entry found for this card")
	case errors.Is(err, cards.ErrUpstream):
		// The joined error names the failing upstream (Vision OCR vs a card
		// catalog); surface it in the server log since the client envelope
		// deliberately stays generic.
		log.Printf("card scan upstream failure: %v", err)
		Error(c, http.StatusBadGateway, CodeUpstreamError, "the card catalog is unreachable, try again")
	case err != nil:
		log.Printf("card scan internal failure: %v", err)
		Error(c, http.StatusInternalServerError, CodeInternal, "scan failed")
	default:
		c.JSON(http.StatusOK, cardScanResponse{
			Game:       res.Game,
			ExternalID: res.ExternalID,
			Name:       res.Name,
			CardType:   res.CardType,
			Race:       res.Race,
			Artist:     res.Artist,
			SetCode:    res.SetCode,
			SetName:    res.SetName,
			Price:      res.Price,
			CoverPath:  res.CoverPath,
		})
	}
}

// add handles POST /api/cards/add — persist a scan result: upsert the shared
// Card row and create the user's CARD tracking item. Returns the shared card
// plus the new tracking item (mirrors the game add flow).
func (h *cardsHandler) add(c *gin.Context) {
	var req cardAddRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		Error(c, http.StatusBadRequest, CodeInvalidRequest, "request body must be JSON with game, externalId, and name")
		return
	}
	req.Game = strings.ToUpper(strings.TrimSpace(req.Game))
	if (req.Game != cards.GameYugioh && req.Game != cards.GamePokemon) ||
		strings.TrimSpace(req.ExternalID) == "" || strings.TrimSpace(req.Name) == "" {
		Error(c, http.StatusBadRequest, CodeInvalidRequest, "game must be YUGIOH or POKEMON with a non-empty externalId and name")
		return
	}
	// coverPath is trusted only within the scan cache's namespace: the Card row
	// is shared across users, so an arbitrary client string must not become
	// another user's artwork path.
	if req.CoverPath != "" && !strings.HasPrefix(req.CoverPath, "card/") {
		req.CoverPath = ""
	}

	card, item, err := h.svc.Add(c.Request.Context(), CurrentUserID(c), cards.ScanResult{
		Game:       req.Game,
		ExternalID: req.ExternalID,
		Name:       req.Name,
		CardType:   req.CardType,
		Race:       req.Race,
		Artist:     req.Artist,
		SetCode:    req.SetCode,
		SetName:    req.SetName,
		Price:      req.Price,
		CoverPath:  req.CoverPath,
	}, req.Status)
	switch {
	case errors.Is(err, cards.ErrInvalidStatus):
		Error(c, http.StatusBadRequest, CodeInvalidRequest, "status must be OWNED for cards")
	case errors.Is(err, cards.ErrAlreadyTracked):
		c.JSON(http.StatusConflict, gin.H{
			"error":   CodeAlreadyTracked,
			"message": "you already track this card",
			"item":    toItemResponse(item),
		})
	case err != nil:
		Error(c, http.StatusInternalServerError, CodeInternal, "adding card failed")
	default:
		c.JSON(http.StatusCreated, gin.H{
			"card": toCardResponse(card),
			"item": toItemResponse(item),
		})
	}
}
