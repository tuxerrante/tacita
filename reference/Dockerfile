# The frozen reference environment: linux/amd64, Go 1.27.0, and Git 2.55.0
# built from the kernel.org release tarball, whose checksum is verified against
# the published sha256sums list. Git's default build includes its Rust core,
# so Cargo is installed for the build stage only.
FROM golang:1.27.0-trixie@sha256:df98008ecd2b0ecc9f0a94d1b07e3564a9c92b555369b33d9b5f60d0765b2db7 AS git
ARG GIT_VERSION=2.55.0
RUN apt-get update \
 && apt-get install -y --no-install-recommends cargo gettext libcurl4-gnutls-dev libexpat1-dev libssl-dev rustc xz-utils zlib1g-dev \
 && rm -rf /var/lib/apt/lists/*
RUN curl -fsSL "https://www.kernel.org/pub/software/scm/git/git-${GIT_VERSION}.tar.xz" -o /tmp/git.tar.xz \
 && curl -fsSL "https://www.kernel.org/pub/software/scm/git/sha256sums.asc" -o /tmp/sha256sums.asc \
 && grep " git-${GIT_VERSION}.tar.xz\$" /tmp/sha256sums.asc | sed "s#git-${GIT_VERSION}.tar.xz#/tmp/git.tar.xz#" | sha256sum -c - \
 && tar -xJf /tmp/git.tar.xz -C /tmp \
 && make -C "/tmp/git-${GIT_VERSION}" -j4 prefix=/usr/local NO_TCLTK=1 NO_PERL=1 NO_PYTHON=1 all install \
 && rm -rf /tmp/git*

FROM golang:1.27.0-trixie@sha256:df98008ecd2b0ecc9f0a94d1b07e3564a9c92b555369b33d9b5f60d0765b2db7
COPY --from=git /usr/local /usr/local
ENV PATH=/usr/local/bin:$PATH
RUN git --version | grep -qx "git version 2.55.0" && go version | grep -q "go1.27.0 linux/amd64"
