package core

import (
	"context"
	"errors"
	"testing"
)

func TestAutomatedPhoneLoginRequiresPrivatePhoneAndOTPProvider(t *testing.T) {
	valid := LoginRequest{
		Mode:  LoginModePhone,
		Phone: "01000000000",
		ReadOTP: func(context.Context) (string, error) {
			return "000000", nil
		},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid phone request rejected: %v", err)
	}
	for _, request := range []LoginRequest{
		{Mode: LoginModePhone},
		{Mode: LoginModePhone, Phone: "01000000000"},
		{Mode: LoginModePhone, Phone: "not-a-phone", ReadOTP: valid.ReadOTP},
	} {
		if err := request.Validate(); !errors.Is(err, ErrInvalidLoginRequest) {
			t.Fatalf("request %#v error = %v, want %v", request, err, ErrInvalidLoginRequest)
		}
	}
}

func TestQRLoginRejectsPhoneSecrets(t *testing.T) {
	request := LoginRequest{
		Mode:  LoginModeQR,
		Phone: "01000000000",
		ReadOTP: func(context.Context) (string, error) {
			return "000000", nil
		},
	}
	if err := request.Validate(); !errors.Is(err, ErrInvalidLoginRequest) {
		t.Fatalf("error = %v, want %v", err, ErrInvalidLoginRequest)
	}
}

func TestQRLoginAllowsOneExplicitPresentationChannel(t *testing.T) {
	presenter := func(context.Context, QRLoginLink) error { return nil }
	if err := (LoginRequest{Mode: LoginModeQR, PresentQRLink: presenter}).Validate(); err != nil {
		t.Fatalf("valid link presenter rejected: %v", err)
	}
	request := LoginRequest{Mode: LoginModeQR, QROutputPath: "synthetic.png", PresentQRLink: presenter}
	if err := request.Validate(); !errors.Is(err, ErrInvalidLoginRequest) {
		t.Fatalf("multiple QR presentation channels error = %v, want %v", err, ErrInvalidLoginRequest)
	}
}
