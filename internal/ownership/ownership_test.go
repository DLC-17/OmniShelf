package ownership

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidate(t *testing.T) {
	cases := []struct {
		name    string
		typ     string
		formats []string
		wantErr error
	}{
		{"music vinyl+cd", TypeMusic, []string{FormatVinyl, FormatCD}, nil},
		{"music single", TypeMusic, []string{FormatCD}, nil},
		{"music empty", TypeMusic, nil, nil},
		{"music unknown format", TypeMusic, []string{"Cassette"}, ErrInvalidFormat},
		{"unsupported type", "BOOK", []string{FormatCD}, ErrNotSupported},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := Validate(tc.typ, tc.formats)
			if tc.wantErr == nil {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, tc.wantErr)
		})
	}
}

func TestNormalizeAndSplit(t *testing.T) {
	// Normalize dedupes and orders by the allowed order (Vinyl before CD).
	assert.Equal(t, "Vinyl,CD", Normalize(TypeMusic, []string{"CD", "Vinyl", "CD"}))
	assert.Equal(t, "", Normalize(TypeMusic, nil))
	assert.Equal(t, "", Normalize("BOOK", []string{"CD"}), "unsupported type yields empty")

	assert.Equal(t, []string{"Vinyl", "CD"}, Split("Vinyl,CD"))
	assert.Equal(t, []string{}, Split(""))
	assert.Equal(t, []string{}, Split("   "))
}

func TestAllowedFormats(t *testing.T) {
	f, ok := AllowedFormats(TypeMusic)
	require.True(t, ok)
	assert.Equal(t, []string{FormatVinyl, FormatCD}, f)

	_, ok = AllowedFormats("BOOK")
	assert.False(t, ok)
}
