//! FlowControl core daemon (`flowd`).

pub mod generated {
    pub mod fctrl_v1 {
        // fctrl.v1.rs ends with `include!("fctrl.v1.tonic.rs")`, so this single
        // include pulls in both the message types and the tonic service stubs.
        include!("generated/fctrl.v1.rs");
    }
}
