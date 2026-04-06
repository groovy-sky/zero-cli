// No-op ctrlc stub for wasm32 targets where signal handling is unavailable.

pub use std::io::Error as SignalError;

#[derive(Debug, Clone, Copy, PartialEq)]
pub enum Signal {
    CtrlC,
    Term,
    Other(i32),
}

#[derive(Debug)]
pub enum Error {
    NoSuchSignal(Signal),
    MultipleHandlers,
    System(std::io::Error),
}

impl std::fmt::Display for Error {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(f, "ctrlc stub: {:?}", self)
    }
}

impl std::error::Error for Error {}

/// No-op: signal handlers are not supported on wasm32.
pub fn set_handler<F>(_handler: F) -> Result<(), Error>
where
    F: Fn() + 'static + Send,
{
    Ok(())
}
