/// Error returned by [`get_timezone`].
#[derive(Debug)]
#[non_exhaustive]
pub enum GetTimezoneError {
    /// An I/O error occurred reading timezone data.
    IoError(std::io::Error),
    /// The operating system returned an error.
    OsError,
    /// The timezone string could not be parsed.
    FailedParsingString,
}

impl std::fmt::Display for GetTimezoneError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            Self::IoError(e) => write!(f, "io error: {e}"),
            Self::OsError => write!(f, "os error"),
            Self::FailedParsingString => write!(f, "failed parsing timezone string"),
        }
    }
}

impl std::error::Error for GetTimezoneError {
    fn source(&self) -> Option<&(dyn std::error::Error + 'static)> {
        match self {
            Self::IoError(e) => Some(e),
            _ => None,
        }
    }
}

/// Returns the name of the system's current IANA timezone.
///
/// This stub reads `/etc/localtime` (a symlink on both Linux and macOS) to
/// determine the timezone, avoiding a dependency on CoreFoundation so that the
/// crate can be cross-compiled from Linux to macOS targets.
pub fn get_timezone() -> Result<String, GetTimezoneError> {
    let path = std::fs::read_link("/etc/localtime").map_err(GetTimezoneError::IoError)?;

    let s = path
        .to_str()
        .ok_or(GetTimezoneError::FailedParsingString)?;

    // The symlink target looks like:
    //   /usr/share/zoneinfo/America/New_York   (Linux)
    //   /var/db/timezone/zoneinfo/America/New_York  (macOS)
    s.find("zoneinfo/")
        .map(|i| s[i + "zoneinfo/".len()..].to_owned())
        .ok_or(GetTimezoneError::FailedParsingString)
}
