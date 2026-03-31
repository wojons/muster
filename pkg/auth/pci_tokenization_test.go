package auth

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPANValidator_ValidatePAN(t *testing.T) {
	validator := NewPANValidator()

	tests := []struct {
		name    string
		pan     string
		wantErr bool
		errType error
	}{
		{
			name:    "valid visa PAN",
			pan:     "4532015112830366",
			wantErr: false,
		},
		{
			name:    "valid mastercard PAN",
			pan:     "5425233430109903",
			wantErr: false,
		},
		{
			name:    "valid amex PAN",
			pan:     "374245455400126",
			wantErr: false,
		},
		{
			name:    "PAN with spaces",
			pan:     "4532 0151 1283 0366",
			wantErr: false,
		},
		{
			name:    "PAN with dashes",
			pan:     "4532-0151-1283-0366",
			wantErr: false,
		},
		{
			name:    "PAN too short",
			pan:     "12345678901",
			wantErr: true,
			errType: ErrPANTooShort,
		},
		{
			name:    "PAN too long",
			pan:     "12345678901234567890",
			wantErr: true,
			errType: ErrPANTooLong,
		},
		{
			name:    "invalid checksum",
			pan:     "4532015112830367",
			wantErr: true,
			errType: ErrInvalidPANChecksum,
		},
		{
			name:    "non-numeric PAN",
			pan:     "453201511283036a",
			wantErr: true,
			errType: ErrInvalidPAN,
		},
		{
			name:    "empty PAN",
			pan:     "",
			wantErr: true,
			errType: ErrInvalidPAN,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidatePAN(tt.pan)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errType != nil {
					assert.ErrorIs(t, err, tt.errType)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestPANValidator_DetectCardNetwork(t *testing.T) {
	validator := NewPANValidator()

	tests := []struct {
		name     string
		pan      string
		expected CardNetwork
	}{
		{
			name:     "Visa starting with 4",
			pan:      "4532015112830366",
			expected: CardNetworkVisa,
		},
		{
			name:     "Visa long",
			pan:      "4916338506082832",
			expected: CardNetworkVisa,
		},
		{
			name:     "Mastercard 51-55",
			pan:      "5425233430109903",
			expected: CardNetworkMastercard,
		},
		{
			name:     "Mastercard 2221-2720",
			pan:      "2223000048410010",
			expected: CardNetworkMastercard,
		},
		{
			name:     "American Express 34",
			pan:      "374245455400126",
			expected: CardNetworkAmex,
		},
		{
			name:     "American Express 37",
			pan:      "378282246310005",
			expected: CardNetworkAmex,
		},
		{
			name:     "Discover 6011",
			pan:      "6011111111111117",
			expected: CardNetworkDiscover,
		},
		{
			name:     "Discover 65",
			pan:      "6500000000000002",
			expected: CardNetworkDiscover,
		},
		{
			name:     "JCB",
			pan:      "3530111333300000",
			expected: CardNetworkJCB,
		},
		{
			name:     "Diners Club 300-305",
			pan:      "30569309025904",
			expected: CardNetworkDiners,
		},
		{
			name:     "Diners Club 36",
			pan:      "36000000000008",
			expected: CardNetworkDiners,
		},
		{
			name:     "Unknown network",
			pan:      "1234567890123456",
			expected: CardNetworkUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			network := validator.DetectCardNetwork(tt.pan)
			assert.Equal(t, tt.expected, network)
		})
	}
}

func TestPANValidator_MaskPAN(t *testing.T) {
	validator := NewPANValidator()

	tests := []struct {
		name     string
		pan      string
		expected string
		wantErr  bool
	}{
		{
			name:     "standard 16 digit PAN",
			pan:      "4532015112830366",
			expected: "****-****-****-0366",
			wantErr:  false,
		},
		{
			name:     "15 digit AMEX",
			pan:      "374245455400126",
			expected: "****-****-***0-126",
			wantErr:  false,
		},
		{
			name:     "PAN with spaces",
			pan:      "4532 0151 1283 0366",
			expected: "****-****-****-0366",
			wantErr:  false,
		},
		{
			name:     "PAN with dashes",
			pan:      "4532-0151-1283-0366",
			expected: "****-****-****-0366",
			wantErr:  false,
		},
		{
			name:    "PAN too short",
			pan:     "123",
			wantErr: true,
		},
		{
			name:    "empty PAN",
			pan:     "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			masked, err := validator.MaskPAN(tt.pan)
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, ErrMaskingFailed)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, masked)
			}
		})
	}
}

func TestInMemoryTokenStore(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryTokenStore()

	t.Run("Store and Retrieve", func(t *testing.T) {
		token := "pcit_test123"
		pan := "4532015112830366"
		metadata := map[string]string{
			"merchant_id": "test-merchant",
			"customer_id": "cust-123",
		}
		expiresAt := time.Now().Add(24 * time.Hour)

		err := store.Store(ctx, token, pan, metadata, expiresAt)
		require.NoError(t, err)

		retrievedPAN, retrievedMeta, retrievedExpiresAt, err := store.Retrieve(ctx, token)
		require.NoError(t, err)
		assert.Equal(t, pan, retrievedPAN)
		assert.Equal(t, metadata["merchant_id"], retrievedMeta["merchant_id"])
		assert.Equal(t, metadata["customer_id"], retrievedMeta["customer_id"])
		assert.WithinDuration(t, expiresAt, retrievedExpiresAt, time.Second)
	})

	t.Run("Retrieve Non-existent Token", func(t *testing.T) {
		_, _, _, err := store.Retrieve(ctx, "pcit_nonexistent")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrTokenNotFound)
	})

	t.Run("Delete Token", func(t *testing.T) {
		token := "pcit_delete123"
		pan := "4532015112830366"

		err := store.Store(ctx, token, pan, nil, time.Time{})
		require.NoError(t, err)

		err = store.Delete(ctx, token)
		require.NoError(t, err)

		_, _, _, err = store.Retrieve(ctx, token)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrTokenNotFound)
	})

	t.Run("Expired Token", func(t *testing.T) {
		token := "pcit_expired"
		pan := "4532015112830366"
		expiresAt := time.Now().Add(-1 * time.Hour)

		err := store.Store(ctx, token, pan, nil, expiresAt)
		require.NoError(t, err)

		_, _, _, err = store.Retrieve(ctx, token)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrTokenExpired)
	})

	t.Run("Exists Check", func(t *testing.T) {
		token := "pcit_exists"
		pan := "4532015112830366"

		exists, err := store.Exists(ctx, token)
		require.NoError(t, err)
		assert.False(t, exists)

		err = store.Store(ctx, token, pan, nil, time.Now().Add(1*time.Hour))
		require.NoError(t, err)

		exists, err = store.Exists(ctx, token)
		require.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("Concurrent Access", func(t *testing.T) {
		done := make(chan bool)
		numGoroutines := 100

		for i := 0; i < numGoroutines; i++ {
			go func(id int) {
				token := "pcit_concurrent"
				pan := "4532015112830366"
				_ = store.Store(ctx, token, pan, nil, time.Time{})
				_, _, _, _ = store.Retrieve(ctx, token)
				_ = store.Delete(ctx, token)
				done <- true
			}(i)
		}

		for i := 0; i < numGoroutines; i++ {
			<-done
		}
	})
}

func TestTokenizer_Tokenize(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryTokenStore()
	tokenizer, err := NewTokenizer(store, []byte("test-hmac-key-32-bytes-long!!"))
	require.NoError(t, err)
	defer func() { _ = tokenizer.Close() }()

	t.Run("Valid PAN Tokenization", func(t *testing.T) {
		req := &TokenizationRequest{
			PAN:        "4532015112830366",
			MerchantID: "test-merchant",
		}

		resp, err := tokenizer.Tokenize(ctx, req)
		require.NoError(t, err)
		assert.NotEmpty(t, resp.Token)
		assert.True(t, strings.HasPrefix(resp.Token, TokenPrefix))
		assert.Equal(t, "0366", resp.PANLast4)
		assert.Equal(t, "****-****-****-0366", resp.PANMasked)
		assert.Equal(t, "VISA", resp.TokenType)
		assert.WithinDuration(t, time.Now(), resp.CreatedAt, time.Second)
	})

	t.Run("Invalid PAN", func(t *testing.T) {
		req := &TokenizationRequest{
			PAN:        "1234567890123456",
			MerchantID: "test-merchant",
		}

		_, err := tokenizer.Tokenize(ctx, req)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidPANChecksum)
	})

	t.Run("Nil Request", func(t *testing.T) {
		_, err := tokenizer.Tokenize(ctx, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be nil")
	})

	t.Run("Tokenization with Expiration", func(t *testing.T) {
		expiresAt := time.Now().Add(1 * time.Hour)
		req := &TokenizationRequest{
			PAN:        "4532015112830366",
			MerchantID: "test-merchant",
			ExpiresAt:  expiresAt,
		}

		resp, err := tokenizer.Tokenize(ctx, req)
		require.NoError(t, err)
		assert.WithinDuration(t, expiresAt, resp.ExpiresAt, time.Second)
	})

	t.Run("Tokenization with Metadata", func(t *testing.T) {
		metadata := map[string]string{
			"customer_id": "cust-123",
			"order_id":    "order-456",
		}
		req := &TokenizationRequest{
			PAN:        "4532015112830366",
			MerchantID: "test-merchant",
			Metadata:   metadata,
		}

		resp, err := tokenizer.Tokenize(ctx, req)
		require.NoError(t, err)
		assert.NotEmpty(t, resp.Token)
	})

	t.Run("Deterministic Tokenization", func(t *testing.T) {
		req := &TokenizationRequest{
			PAN:        "4532015112830366",
			MerchantID: "test-merchant",
		}

		resp1, err := tokenizer.Tokenize(ctx, req)
		require.NoError(t, err)

		resp2, err := tokenizer.Tokenize(ctx, req)
		require.NoError(t, err)

		assert.Equal(t, resp1.Token, resp2.Token)
	})

	t.Run("Different Merchants Get Different Tokens", func(t *testing.T) {
		req1 := &TokenizationRequest{
			PAN:        "4532015112830366",
			MerchantID: "merchant-1",
		}
		req2 := &TokenizationRequest{
			PAN:        "4532015112830366",
			MerchantID: "merchant-2",
		}

		resp1, err := tokenizer.Tokenize(ctx, req1)
		require.NoError(t, err)

		resp2, err := tokenizer.Tokenize(ctx, req2)
		require.NoError(t, err)

		assert.NotEqual(t, resp1.Token, resp2.Token)
	})

	t.Run("Tokenizer Closed", func(t *testing.T) {
		newTokenizer, err := NewTokenizer(NewInMemoryTokenStore(), nil)
		require.NoError(t, err)

		err = newTokenizer.Close()
		require.NoError(t, err)

		req := &TokenizationRequest{
			PAN:        "4532015112830366",
			MerchantID: "test-merchant",
		}

		_, err = newTokenizer.Tokenize(ctx, req)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrTokenizationFailed)
	})
}

func TestTokenizer_Detokenize(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryTokenStore()
	tokenizer, err := NewTokenizer(store, []byte("test-hmac-key-32-bytes-long!!"))
	require.NoError(t, err)
	defer func() { _ = tokenizer.Close() }()

	t.Run("Valid Detokenization", func(t *testing.T) {
		tokenReq := &TokenizationRequest{
			PAN:        "4532015112830366",
			MerchantID: "test-merchant",
		}
		tokenResp, err := tokenizer.Tokenize(ctx, tokenReq)
		require.NoError(t, err)

		detokenReq := &DetokenizationRequest{
			Token:      tokenResp.Token,
			MerchantID: "test-merchant",
		}

		resp, err := tokenizer.Detokenize(ctx, detokenReq)
		require.NoError(t, err)
		assert.Equal(t, "4532015112830366", resp.PAN)
		assert.Equal(t, "****-****-****-0366", resp.PANMasked)
		assert.Equal(t, "VISA", resp.TokenType)
	})

	t.Run("Invalid Token Format", func(t *testing.T) {
		req := &DetokenizationRequest{
			Token:      "invalid-token",
			MerchantID: "test-merchant",
		}

		_, err := tokenizer.Detokenize(ctx, req)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrTokenNotFound)
	})

	t.Run("Merchant ID Mismatch", func(t *testing.T) {
		tokenReq := &TokenizationRequest{
			PAN:        "4532015112830366",
			MerchantID: "merchant-1",
		}
		tokenResp, err := tokenizer.Tokenize(ctx, tokenReq)
		require.NoError(t, err)

		detokenReq := &DetokenizationRequest{
			Token:      tokenResp.Token,
			MerchantID: "merchant-2",
		}

		_, err = tokenizer.Detokenize(ctx, detokenReq)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "merchant ID mismatch")
	})

	t.Run("Nil Request", func(t *testing.T) {
		_, err := tokenizer.Detokenize(ctx, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be nil")
	})

	t.Run("Tokenizer Closed", func(t *testing.T) {
		newTokenizer, err := NewTokenizer(NewInMemoryTokenStore(), nil)
		require.NoError(t, err)

		err = newTokenizer.Close()
		require.NoError(t, err)

		req := &DetokenizationRequest{
			Token:      "pcit_test",
			MerchantID: "test-merchant",
		}

		_, err = newTokenizer.Detokenize(ctx, req)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrDetokenizationFailed)
	})
}

func TestTokenizer_DeleteAndExists(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryTokenStore()
	tokenizer, err := NewTokenizer(store, []byte("test-hmac-key-32-bytes-long!!"))
	require.NoError(t, err)
	defer func() { _ = tokenizer.Close() }()

	t.Run("Delete Token", func(t *testing.T) {
		tokenReq := &TokenizationRequest{
			PAN:        "4532015112830366",
			MerchantID: "test-merchant",
		}
		tokenResp, err := tokenizer.Tokenize(ctx, tokenReq)
		require.NoError(t, err)

		exists, err := tokenizer.TokenExists(ctx, tokenResp.Token)
		require.NoError(t, err)
		assert.True(t, exists)

		err = tokenizer.DeleteToken(ctx, tokenResp.Token)
		require.NoError(t, err)

		exists, err = tokenizer.TokenExists(ctx, tokenResp.Token)
		require.NoError(t, err)
		assert.False(t, exists)
	})
}

func TestRedactPAN(t *testing.T) {
	tests := []struct {
		name     string
		pan      string
		expected string
	}{
		{
			name:     "standard PAN",
			pan:      "4532015112830366",
			expected: "************0366",
		},
		{
			name:     "PAN with spaces",
			pan:      "4532 0151 1283 0366",
			expected: "************0366",
		},
		{
			name:     "short PAN",
			pan:      "123",
			expected: "***",
		},
		{
			name:     "empty PAN",
			pan:      "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := RedactPAN(tt.pan)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestConvenienceFunctions(t *testing.T) {
	t.Run("ValidateCardNumber", func(t *testing.T) {
		err := ValidateCardNumber("4532015112830366")
		require.NoError(t, err)

		err = ValidateCardNumber("1234567890123456")
		require.Error(t, err)
	})

	t.Run("MaskCardNumber", func(t *testing.T) {
		masked, err := MaskCardNumber("4532015112830366")
		require.NoError(t, err)
		assert.Equal(t, "****-****-****-0366", masked)
	})

	t.Run("GetCardNetwork", func(t *testing.T) {
		network := GetCardNetwork("4532015112830366")
		assert.Equal(t, CardNetworkVisa, network)

		network = GetCardNetwork("5425233430109903")
		assert.Equal(t, CardNetworkMastercard, network)
	})
}

func TestTokenizer_ConcurrentTokenization(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryTokenStore()
	tokenizer, err := NewTokenizer(store, nil)
	require.NoError(t, err)
	defer func() { _ = tokenizer.Close() }()

	var wg sync.WaitGroup
	numGoroutines := 50
	results := make(chan *TokenizationResponse, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			req := &TokenizationRequest{
				PAN:        "4532015112830366",
				MerchantID: "test-merchant",
			}
			resp, err := tokenizer.Tokenize(ctx, req)
			if err == nil {
				results <- resp
			}
		}(i)
	}

	wg.Wait()
	close(results)

	count := 0
	for range results {
		count++
	}

	assert.Equal(t, numGoroutines, count)
}

func TestTokenExpiration(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryTokenStore()
	tokenizer, err := NewTokenizer(store, nil)
	require.NoError(t, err)
	defer func() { _ = tokenizer.Close() }()

	t.Run("Expired Token Cannot Be Detokenized", func(t *testing.T) {
		expiresAt := time.Now().Add(100 * time.Millisecond)
		req := &TokenizationRequest{
			PAN:        "4532015112830366",
			MerchantID: "test-merchant",
			ExpiresAt:  expiresAt,
		}

		resp, err := tokenizer.Tokenize(ctx, req)
		require.NoError(t, err)

		time.Sleep(200 * time.Millisecond)

		detokenReq := &DetokenizationRequest{
			Token:      resp.Token,
			MerchantID: "test-merchant",
		}

		_, err = tokenizer.Detokenize(ctx, detokenReq)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrTokenExpired)
	})
}
