package drive_roles

import (
	"errors"
	"fmt"
)

// Code is the stable, machine-readable name of a role or permission failure.
// The UI translates codes; Message is display text only and must never be
// branched on. Every refusal to change content in a shared drive carries
// CodePermissionDenied, whichever rule refused it.
type Code string

const (
	CodeNotSharedDrive    Code = "not_shared_drive"
	CodeNotMember         Code = "not_a_member"
	CodeUserNotFound      Code = "user_not_found"
	CodePermissionDenied  Code = "permission_denied"
	CodeCannotChangeOwner Code = "cannot_change_owner"
	CodeLastManager       Code = "last_manager"
	CodeInvalidRole       Code = "invalid_role"
	CodeRoleUnavailable   Code = "role_unavailable"
)

// Error carries a stable Code and a display message. errors.Is matches on the
// Code, so callers can compare against the sentinels below or any *Error.
type Error struct {
	Code    Code
	Message string
	cause   error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	return ok && t != nil && e != nil && t.Code == e.Code
}

// Sentinels for matching with errors.Is.
var (
	ErrNotSharedDrive   = &Error{Code: CodeNotSharedDrive, Message: "This drive is not shared."}
	ErrNotMember        = &Error{Code: CodeNotMember, Message: "You are not a member of this drive."}
	ErrUserNotFound     = &Error{Code: CodeUserNotFound, Message: "That member could not be found."}
	ErrPermissionDenied = &Error{Code: CodePermissionDenied, Message: "You do not have permission to change members of this drive."}
	ErrViewOnly         = &Error{Code: CodePermissionDenied, Message: "You only have view access to this drive"}
	ErrOwnerImmutable   = &Error{Code: CodeCannotChangeOwner, Message: "The drive owner's role cannot be changed."}
	ErrLastManager      = &Error{Code: CodeLastManager, Message: "A manager cannot step down when they are the last manager."}
	ErrInvalidRole      = &Error{Code: CodeInvalidRole, Message: "That role is not valid for this action."}
	ErrRoleUnavailable  = &Error{Code: CodeRoleUnavailable, Message: "Could not check your role in this drive. Try again when Telegram is reachable."}
)

// NotUploaderError is returned when an editor tries to change a file another
// member uploaded. action names the change ("rename", "move", "delete").
func NotUploaderError(action string) error {
	return &Error{
		Code:    CodePermissionDenied,
		Message: fmt.Sprintf("Only the uploader can %s this file in a shared drive", action),
	}
}

// CodeOf returns the stable code of err, or "" when err carries none.
func CodeOf(err error) Code {
	var role_err *Error
	if errors.As(err, &role_err) && role_err != nil {
		return role_err.Code
	}
	return ""
}

// wrap_error attaches a cause to a sentinel without changing its code.
func wrap_error(sentinel *Error, cause error) error {
	return &Error{Code: sentinel.Code, Message: sentinel.Message, cause: cause}
}
