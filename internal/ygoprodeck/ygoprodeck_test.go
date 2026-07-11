package ygoprodeck

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testSetCode = "LOB-001"

// goodSetInfo is a happy /cardsetsinfo.php answer for testSetCode.
const goodSetInfo = `{"id":89631146,"name":"Blue-Eyes White Dragon",
	"set_name":"Legend of Blue Eyes White Dragon","set_code":"LOB-001",
	"set_rarity":"Ultra Rare","set_price":"62.15"}`

// server stands in for the YGOPRODeck /cardsetsinfo.php + /cardinfo.php pair.
type server struct {
	setInfoBody   string
	setInfoStatus int
	cardBody      string
	cardStatus    int
	gotSetcode    string
	gotID         string
}

func (s *server) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cardsetsinfo.php":
			s.gotSetcode = r.URL.Query().Get("setcode")
			if s.setInfoStatus != 0 {
				w.WriteHeader(s.setInfoStatus)
			}
			_, _ = w.Write([]byte(s.setInfoBody))
		case "/cardinfo.php":
			s.gotID = r.URL.Query().Get("id")
			if s.cardStatus != 0 {
				w.WriteHeader(s.cardStatus)
			}
			_, _ = w.Write([]byte(s.cardBody))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}
}

func newClient(t *testing.T, s *server) *Client {
	t.Helper()
	srv := httptest.NewServer(s.handler())
	t.Cleanup(srv.Close)
	return New(WithBaseURL(srv.URL))
}

func TestCardBySetCode(t *testing.T) {
	tests := []struct {
		name          string
		setInfoBody   string
		setInfoStatus int
		cardBody      string
		cardStatus    int
		want          *Card
		wantErr       error
	}{
		{
			name:        "full card: set-specific price and set name win",
			setInfoBody: goodSetInfo,
			cardBody: `{"data":[{"name":"Blue-Eyes White Dragon","type":"Normal Monster","race":"Dragon",
				"card_images":[{"image_url":"http://img.test/lob-001.jpg"}],
				"card_prices":[{"tcgplayer_price":"12.34"}]}]}`,
			want: &Card{
				Name:     "Blue-Eyes White Dragon",
				Type:     "Normal Monster",
				Race:     "Dragon",
				SetName:  "Legend of Blue Eyes White Dragon",
				Rarity:   "Ultra Rare",
				Price:    62.15,
				ImageURL: "http://img.test/lob-001.jpg",
			},
		},
		{
			name: "zero set price falls back to the tcgplayer price",
			setInfoBody: `{"id":1,"name":"Odd","set_name":"Some Set","set_code":"LOB-001",
				"set_rarity":"Common","set_price":"0.00"}`,
			cardBody: `{"data":[{"name":"Odd","type":"Effect Monster","race":"Fiend",
				"card_prices":[{"tcgplayer_price":"3.21"}]}]}`,
			want: &Card{
				Name: "Odd", Type: "Effect Monster", Race: "Fiend",
				SetName: "Some Set", Rarity: "Common", Price: 3.21,
			},
		},
		{
			name: "unparseable prices and missing images degrade to zero values",
			setInfoBody: `{"id":1,"name":"Odd","set_name":"Some Set","set_code":"LOB-001",
				"set_rarity":"Common","set_price":"n/a"}`,
			cardBody: `{"data":[{"name":"Odd","type":"Effect Monster","race":"Fiend",
				"card_prices":[{"tcgplayer_price":"n/a"}]}]}`,
			want: &Card{
				Name: "Odd", Type: "Effect Monster", Race: "Fiend",
				SetName: "Some Set", Rarity: "Common",
			},
		},
		{
			name:          "unknown set code answers 400 on cardsetsinfo",
			setInfoBody:   `{"error":"No card matching your query was found in the database."}`,
			setInfoStatus: http.StatusBadRequest,
			wantErr:       ErrNotFound,
		},
		{
			name:        "set info without a card id is a miss",
			setInfoBody: `{}`,
			wantErr:     ErrNotFound,
		},
		{
			name:        "empty cardinfo data",
			setInfoBody: goodSetInfo,
			cardBody:    `{"data":[]}`,
			wantErr:     ErrNotFound,
		},
		{
			name:          "server error on cardsetsinfo",
			setInfoBody:   `boom`,
			setInfoStatus: http.StatusInternalServerError,
			wantErr:       ErrUpstream,
		},
		{
			name:        "server error on cardinfo",
			setInfoBody: goodSetInfo,
			cardBody:    `boom`,
			cardStatus:  http.StatusInternalServerError,
			wantErr:     ErrUpstream,
		},
		{
			name:        "malformed cardinfo payload",
			setInfoBody: goodSetInfo,
			cardBody:    `{"data":`,
			wantErr:     ErrUpstream,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &server{
				setInfoBody: tt.setInfoBody, setInfoStatus: tt.setInfoStatus,
				cardBody: tt.cardBody, cardStatus: tt.cardStatus,
			}
			c := newClient(t, s)

			card, err := c.CardBySetCode(context.Background(), testSetCode)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, card)
			assert.Equal(t, testSetCode, s.gotSetcode, "set code must be sent as the setcode query param")
			assert.NotEmpty(t, s.gotID, "cardinfo must be queried by the id cardsetsinfo resolved")
		})
	}
}
