ARG BASE_IMAGE=golang:1.15-alpine
ARG WORKDIR=/opt/nitro/imgdeflator
ARG VIPS_VERSION="8.13.3"


configure:
  FROM $BASE_IMAGE
  WORKDIR $WORKDIR
  COPY --dir .git .git
  RUN git config pull.rebase true \
    && git config remote.origin.prune true \
    && git config branch.main.mergeoptions "--ff-only" \
    && git clone https://github.com/awslabs/git-secrets.git \
    && cd git-secrets \
    && make install \
    && git secrets --install -f \
    && git secrets --register-aws
  SAVE ARTIFACT .git/config AS LOCAL .git/
  SAVE ARTIFACT .git/hooks/* AS LOCAL .git/hooks/

checks:
  BUILD +go-build-dev
  BUILD +go-test

go-build-dev:
  FROM +go-base
  RUN  PKG_CONFIG_PATH=/root/vips go build -ldflags="-s -w" -o imgdeflator imgdeflator.go
  SAVE ARTIFACT imgdeflator

go-base:
  FROM $BASE_IMAGE
  RUN apk --update add --no-cache \
  	git gcc g++ make musl-dev fftw-dev glib-dev expat-dev \
	libjpeg-turbo-dev libpng-dev libwebp-dev giflib-dev librsvg-dev libexif-dev lcms2-dev
  RUN cd /root \
	&& mkdir vips \
	&& wget -qO- "https://github.com/libvips/libvips/releases/download/v${VIPS_VERSION}/vips-${VIPS_VERSION}.tar.gz" | tar -xzf - -C vips --strip-components=1 \
    && cd vips \
    && ./configure \
	    --disable-magickload \
		--without-imagequant \
		--without-tiff \
		--without-orc \
		--without-OpenEXR \
		--without-pdfium \
		--without-poppler \
		--without-matio \
		--without-openslide \
		--without-cfitsio \
		--disable-static \
		--enable-silent-rules \
    && make install-strip
  WORKDIR $WORKDIR
  COPY . .
  RUN go mod download

go-test:
  FROM +go-base
  RUN go test -v --timeout 30s ./...
