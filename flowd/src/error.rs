//! The one domain-error taxonomy, shared by every host.
//!
//! Today `grpc::into_status` classifies errors by lowercasing a `Box<dyn Error>`'s
//! text and substring-matching it. That logic exists only on the native tonic
//! edge — once the store core is reused under Node and the browser, each host
//! would re-derive the same classification and drift. `DomainError` carries an
//! explicit [`Code`] plus the host's original message, so the mapping is written
//! **once** (here) and every edge — native `into_status`, the Node connect-node
//! router, the browser's thrown error — reads the same `Code`.
//!
//! Wired in but not yet consumed: the sqlx→rusqlite port (Phase 1) swaps the
//! store's `Box<dyn Error + Send + Sync>` return type for `DomainError`.
#![allow(dead_code)]

use std::fmt;

/// The transport-neutral classification. Each host maps this to its own status
/// space: native → `tonic::Status`, Node/browser → a Connect/gRPC code.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Code {
    /// The referenced entity does not exist.
    NotFound,
    /// The request is malformed (empty mask, unknown field, bad FTS query, …).
    InvalidArgument,
    /// The request is well-formed but not allowed in the current state
    /// (cycle, wrong parent kind, non-undoable event, referenced node, …).
    FailedPrecondition,
    /// Anything unclassified — a bug or an unexpected DB failure.
    Internal,
}

/// A domain error: a [`Code`] plus the original human-readable message (which may
/// be a SQLite trigger `RAISE(ABORT, …)` string — preserved verbatim so nothing
/// is lost across a host boundary).
#[derive(Debug, Clone)]
pub struct DomainError {
    pub code: Code,
    pub message: String,
}

impl DomainError {
    pub fn new(code: Code, message: impl Into<String>) -> Self {
        Self {
            code,
            message: message.into(),
        }
    }
    pub fn not_found(message: impl Into<String>) -> Self {
        Self::new(Code::NotFound, message)
    }
    pub fn invalid_argument(message: impl Into<String>) -> Self {
        Self::new(Code::InvalidArgument, message)
    }
    pub fn failed_precondition(message: impl Into<String>) -> Self {
        Self::new(Code::FailedPrecondition, message)
    }
    pub fn internal(message: impl Into<String>) -> Self {
        Self::new(Code::Internal, message)
    }

    /// Classify a raw host/SQLite error message (e.g. a trigger `RAISE(ABORT)`
    /// string, or an FTS `MATCH` syntax error) into a [`Code`], preserving the
    /// exact taxonomy the gRPC edge string-matched before this type existed.
    pub fn from_db_message(message: impl Into<String>) -> Self {
        let message = message.into();
        Self {
            code: classify(&message),
            message,
        }
    }
}

/// The single source of truth for message → [`Code`], lifted verbatim from the
/// old `grpc::into_status`. Store logic uses the explicit constructors above for
/// its own errors; only opaque DB errors flow through here.
pub fn classify(message: &str) -> Code {
    let m = message.to_lowercase();
    if m.contains("not found") {
        Code::NotFound
    } else if m.contains("update_mask cannot be empty")
        || m.contains("unknown update_mask")
        || m.contains("requires a target kind")
        || m.contains("invalid agent result")
        || m.contains("syntax error") // malformed FTS query from Search
        || m.contains("fts5")
    {
        Code::InvalidArgument
    } else if m.contains("invalid parent kind")
        || m.contains("cross-project")
        || m.contains("children invalid")
        || m.contains("its own parent")
        || m.contains("would create a cycle")
        || m.contains("has dependents")
        || m.contains("work package") // "cannot promote or demote a work package"
        || m.contains("cannot demote")
        || m.contains("cannot undo")
    {
        Code::FailedPrecondition
    } else {
        Code::Internal
    }
}

impl fmt::Display for DomainError {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        f.write_str(&self.message)
    }
}

impl std::error::Error for DomainError {}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn classify_matches_the_old_taxonomy() {
        assert_eq!(classify("node not found"), Code::NotFound);
        assert_eq!(
            classify("update_mask cannot be empty"),
            Code::InvalidArgument
        );
        assert_eq!(
            classify("fts5: syntax error near \"(\""),
            Code::InvalidArgument
        );
        assert_eq!(classify("would create a cycle"), Code::FailedPrecondition);
        assert_eq!(
            classify("cannot promote or demote a work package"),
            Code::FailedPrecondition
        );
        assert_eq!(
            classify("cannot undo event kind 7"),
            Code::FailedPrecondition
        );
        assert_eq!(classify("disk I/O error"), Code::Internal);
    }

    #[test]
    fn explicit_constructors_carry_their_code() {
        assert_eq!(DomainError::not_found("x").code, Code::NotFound);
        assert_eq!(
            DomainError::from_db_message("invalid parent kind").code,
            Code::FailedPrecondition
        );
    }
}
