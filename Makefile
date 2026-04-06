BIN_DIR      := bin
WASM_OUT     := wasm/uutils.wasm
COREUTILS_DIR := vendor/coreutils

# Prefer the rustup-managed cargo if available (system cargo may be too old).
export PATH := $(HOME)/.cargo/bin:$(PATH)
CARGO := cargo

UUTILS_FEATURES := ls,cat,cp,mv,rm,mkdir,touch,pwd,head,nl,wc,uniq,tr,tee,sha256sum,uname,test,printf,echo

PLATFORMS := \
	linux/amd64 \
	linux/arm64 \
	darwin/amd64 \
	darwin/arm64 \
	windows/amd64

.PHONY: all wasm build clean

all: wasm build

# ── Build uutils.wasm from the git submodule ────────────────────────────────
wasm: $(WASM_OUT)

$(WASM_OUT): $(COREUTILS_DIR)/Cargo.toml Makefile
	$(CARGO) build \
		--manifest-path $(COREUTILS_DIR)/Cargo.toml \
		--target wasm32-wasip1 \
		--no-default-features \
		--features "$(UUTILS_FEATURES)" \
		--bin coreutils \
		--release
	cp $(COREUTILS_DIR)/target/wasm32-wasip1/release/coreutils.wasm $(WASM_OUT)

# ── Cross-compile the MCP server binary ─────────────────────────────────────
build: $(WASM_OUT)
	@mkdir -p $(BIN_DIR)
	@echo "Most useful in VS Code terminal workflows: $(UUTILS_FEATURES)"
	@for platform in $(PLATFORMS); do \
		GOOS=$${platform%/*}; \
		GOARCH=$${platform#*/}; \
		case $$GOARCH in \
			amd64) RUST_ARCH="x86_64"; ;; \
			arm64) RUST_ARCH="aarch64"; ;; \
		esac; \
		case $$GOOS in \
			linux) \
				TRIPLE="$$RUST_ARCH-unknown-linux-musl"; \
				BUILD_CMD="$(CARGO) build"; \
				EXT=""; ;; \
			darwin) \
				TRIPLE="$$RUST_ARCH-apple-darwin"; \
				BUILD_CMD="$(CARGO) zigbuild"; \
				EXT=""; ;; \
			windows) \
				TRIPLE="$$RUST_ARCH-pc-windows-gnu"; \
				BUILD_CMD="$(CARGO) zigbuild"; \
				EXT=".exe"; ;; \
		esac; \
		OUT="$(BIN_DIR)/wasm-shell-mcp-$$GOOS-$$GOARCH$$EXT"; \
		echo "Building $$OUT ($$TRIPLE)"; \
		$$BUILD_CMD --release --target $$TRIPLE; \
		cp target/$$TRIPLE/release/wasm-shell-mcp$$EXT $$OUT; \
	done

# ── Initialise submodule ─────────────────────────────────────────────────────
$(COREUTILS_DIR)/Cargo.toml:
	git submodule update --init --recursive

clean:
	rm -f $(WASM_OUT)
	$(CARGO) clean
