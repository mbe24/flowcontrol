#![no_main]
//! Fuzz the one untrusted-input path in flowd: full-text `search`. Arbitrary
//! query text (from an agent or user) must never panic — a malformed FTS query
//! is an Err (mapped to InvalidArgument at the gRPC layer), never a crash.
//!
//! Run (needs nightly + cargo-fuzz + a C/C++ toolchain for libFuzzer):
//!   cd flowd && cargo +nightly fuzz run search_never_panics

use flowd::db;
use flowd::store::{SqliteStore, Store};
use libfuzzer_sys::fuzz_target;
use once_cell::sync::Lazy;

static RT: Lazy<tokio::runtime::Runtime> =
    Lazy::new(|| tokio::runtime::Runtime::new().unwrap());

// One seeded in-memory store, reused across iterations (cheap per-iteration cost).
static STORE: Lazy<SqliteStore> = Lazy::new(|| {
    RT.block_on(async {
        let pool = db::open(":memory:").await.unwrap();
        db::seed(&pool).await.unwrap();
        SqliteStore::from_pool(pool)
    })
});

fuzz_target!(|data: &[u8]| {
    if let Ok(query) = std::str::from_utf8(data) {
        // Only assertion: no panic. Ok(results) or Err(bad query) are both fine.
        let _ = RT.block_on(STORE.search("prj-travel", query, 10));
    }
});
