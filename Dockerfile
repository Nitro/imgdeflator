FROM golang:1.11-alpine3.9 as builder

# Inspired from https://github.com/DarthSim/imgproxy/blob/a344a47f0fa4b492e0a54db047a53991c05419ac/Dockerfile
# Note: All the dependencies have been adjusted one way or the other

ENV IMAGEMAGICK_VERSION "7.0.8-45"
ENV VIPS_VERSION "8.7.4"

# Install dependencies
RUN apk --update add --no-cache \
	git gcc g++ make musl-dev fftw-dev glib-dev expat-dev \
	libjpeg-turbo-dev libpng-dev libwebp-dev giflib-dev librsvg-dev libexif-dev lcms2-dev

# Build ImageMagick
RUN cd /root \
	&& mkdir ImageMagick \
	&& wget -qO- "https://imagemagick.org/download/ImageMagick-${IMAGEMAGICK_VERSION}.tar.gz" | tar -xzf - -C ImageMagick --strip-components=1 \
	&& cd ImageMagick \
	&& MKDIR_P="/bin/mkdir -p" ./configure \
		--enable-silent-rules \
		--disable-static \
		--disable-openmp \
		--disable-deprecated \
		--disable-docs \
		--with-threads \
		--without-magick-plus-plus \
		--without-utilities \
		--without-perl \
		--without-bzlib \
		--without-dps \
		--without-freetype \
		--without-jbig \
		--without-jpeg \
		--without-lcms \
		--without-lzma \
		--without-png \
		--without-tiff \
		--without-wmf \
		--without-xml \
		--without-webp \
		# See https://github.com/ImageMagick/ImageMagick/issues/1572
		--disable-dependency-tracking \
	&& make install-strip

# Build vips
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

ADD . /root/imgdeflator

# Build imgdeflator
RUN cd /root/imgdeflator \
	&& go build && echo $?

# Copy compiled libs in /root/libs to easily add them in the final image
RUN cd /root \
	&& mkdir libs \
	&& ldd /root/imgdeflator/imgdeflator | grep /usr/local/lib/ | awk '{print $3}' | xargs -I '{}' cp '{}' libs/


################
# Actual image #
################
FROM alpine:3.9

# Set up s6
RUN wget -qO- https://github.com/just-containers/skaware/releases/download/v1.21.7/s6-2.7.2.0-linux-amd64-bin.tar.gz | tar -xvzf -
ADD docker/s6 /etc

RUN apk --update add --no-cache \
	ca-certificates fftw glib expat libjpeg-turbo libpng \
	libwebp giflib librsvg libgsf libexif lcms2

COPY --from=builder /root/imgdeflator/imgdeflator /imgdeflator/imgdeflator
COPY --from=builder /root/libs/* /usr/local/lib/

CMD ["/bin/s6-svscan", "/etc/services"]

EXPOSE 8080