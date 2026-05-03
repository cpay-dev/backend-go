FROM postgres:18 AS builder

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        build-essential \
        ca-certificates \
        curl \
        postgresql-server-dev-18 \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /tmp/pg-ext-ulid
RUN curl -fsSL https://github.com/cpay-dev/pg-ext-ulid/archive/refs/heads/main.tar.gz \
    | tar -xz --strip-components=1
RUN make \
    && make install DESTDIR=/tmp/ulid-install

FROM postgres:18

COPY --from=builder /tmp/ulid-install/usr/lib/postgresql/18/lib/ulid.so /usr/lib/postgresql/18/lib/ulid.so
COPY --from=builder /tmp/ulid-install/usr/share/postgresql/18/extension/ulid.control /usr/share/postgresql/18/extension/ulid.control
COPY --from=builder /tmp/ulid-install/usr/share/postgresql/18/extension/ulid--0.0.1.sql /usr/share/postgresql/18/extension/ulid--0.0.1.sql
