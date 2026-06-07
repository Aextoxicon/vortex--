FROM rust:1.86 AS chef
RUN cargo install cargo-chef --locked

FROM chef AS planner
WORKDIR /app
COPY . .
RUN cargo chef prepare --recipe-path recipe.json

FROM chef AS builder
WORKDIR /app
COPY --from=planner /app/recipe.json recipe.json
RUN cargo chef cook --release --recipe-path recipe.json
COPY . .
RUN cargo build --release

FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates libpq5 \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd -r vortex && useradd -r -g vortex -d /app -s /sbin/nologin vortex

WORKDIR /app

COPY --from=builder /app/target/release/vortex-- /app/vortex

RUN chown -R vortex:vortex /app

USER vortex

EXPOSE 9178

ENV RUST_LOG=info

ENTRYPOINT ["/app/vortex"]
