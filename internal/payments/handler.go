package payments

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"

	apperrors "github.com/BUCC-Backend-Team/payment_service_4/internal/errors"
)

const MaxBodyBytes = 1 << 20

type apiHandler struct {
	service *Service
}

func NewHandler(service *Service) *apiHandler {
	return &apiHandler{service: service}
}

type createPaymentBody struct {
	OrderID       string `json:"orderId"`
	Amount        int64  `json:"amount"`
	Currency      string `json:"currency"`
	CustomerEmail string `json:"customerEmail"`
	ReturnURL     string `json:"returnUrl"`
}

func (h *apiHandler) Create(w http.ResponseWriter, r *http.Request) {
	idemKey := r.Header.Get("Idempotency-Key")
	if strings.TrimSpace(idemKey) == "" {
		WriteError(w, apperrors.New(http.StatusBadRequest, "missing_idempotency_key", "Idempotency-Key header is required"))
		return
	}

	var body createPaymentBody
	if !decodeJSONBody(w, r, &body) {
		return
	}

	req, err := validatedCreateRequest(body)
	if err != nil {
		WriteError(w, err)
		return
	}

	result, replayed, createErr := h.service.Create(r.Context(), req, strings.TrimSpace(idemKey))
	if createErr != nil {
		WriteError(w, createErr)
		return
	}

	status := http.StatusCreated
	headers := map[string]string{}
	if replayed {
		headers["Idempotent-Replayed"] = "true"
	}
	WriteJSON(w, status, result, headers)
}

func (h *apiHandler) Get(w http.ResponseWriter, r *http.Request) {
	paymentID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		WriteError(w, apperrors.New(http.StatusBadRequest, "invalid_payment_id", "Payment id must be a UUID"))
		return
	}

	payment, err := h.service.Get(r.Context(), paymentID)
	if err != nil {
		WriteError(w, err)
		return
	}

	WriteJSON(w, http.StatusOK, payment, map[string]string{})
}

func validatedCreateRequest(body createPaymentBody) (CreateRequest, error) {
	orderID := strings.TrimSpace(body.OrderID)
	if orderID == "" {
		return CreateRequest{}, apperrors.New(http.StatusBadRequest, "invalid_order_id", "orderId is required")
	}
	if body.Amount <= 0 {
		return CreateRequest{}, apperrors.New(http.StatusBadRequest, "invalid_amount", "amount must be a positive integer of kobo")
	}

	currency := strings.ToUpper(strings.TrimSpace(body.Currency))
	if currency == "" {
		currency = "NGN"
	}
	if currency != "NGN" {
		return CreateRequest{}, apperrors.New(http.StatusBadRequest, "invalid_currency", "Only NGN is supported")
	}

	email := strings.TrimSpace(body.CustomerEmail)
	if !validEmail(email) {
		return CreateRequest{}, apperrors.New(http.StatusBadRequest, "invalid_customer_email", "customerEmail must be a valid address")
	}

	returnURL := strings.TrimSpace(body.ReturnURL)
	parsed, err := url.Parse(returnURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return CreateRequest{}, apperrors.New(http.StatusBadRequest, "invalid_return_url", "returnUrl must be an http(s) URL")
	}

	return CreateRequest{
		OrderID:       orderID,
		Amount:        body.Amount,
		Currency:      currency,
		CustomerEmail: email,
		ReturnURL:     returnURL,
	}, nil
}

func validEmail(email string) bool {
	at := strings.LastIndex(email, "@")
	return at > 0 && at < len(email)-1 && !strings.ContainsAny(email, " \t\r\n")
}

func decodeJSONBody(w http.ResponseWriter, r *http.Request, into any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(into); err != nil {
		WriteError(w, apperrors.New(http.StatusBadRequest, "invalid_request_body", "Request body must be valid JSON"))
		return false
	}
	return true
}

func WriteJSON(w http.ResponseWriter, status int, body any, extra map[string]string) {
	for key, value := range extra {
		w.Header().Set(key, value)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func WriteError(w http.ResponseWriter, err error) {
	appErr, ok := err.(*apperrors.AppError)
	if !ok {
		appErr = apperrors.New(http.StatusInternalServerError, "internal_error", "Internal server error")
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(appErr.HTTPStatus())
	_ = json.NewEncoder(w).Encode(map[string]string{"error": appErr.Code})
}
