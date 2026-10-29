# syntax=docker/dockerfile:1

ARG PHP_VERSION=8.5
ARG PHP_VARIANT=fpm

ARG PHP_VERSION_COMPOSER=2.11
ARG PHP_VERSION_IGBINARY=^3.2.17RC1@RC
ARG PHP_VERSION_IMAGICK=^3.8.1
ARG PHP_VERSION_REDIS=^6.3.0
ARG PHP_VERSION_SQLSRV=^5.13.3
ARG PHP_VERSION_XDEBUG=^3.5.3
ARG PHP_VERSION_YAML=^2.3.0

ARG PHP_BUILD_IMAGICK=0
ARG PHP_BUILD_MYSQL=0
ARG PHP_BUILD_PGSQL=0
ARG PHP_BUILD_REDIS=0
ARG PHP_BUILD_SQLSRV=0
ARG PHP_INSTALL_FFMPEG=0

FROM php:${PHP_VERSION}-${PHP_VARIANT}-alpine AS php

LABEL org.opencontainers.image.title="XenForo"
LABEL org.opencontainers.image.description="A PHP runtime for XenForo"
LABEL org.opencontainers.image.vendor="XenForo Ltd."
LABEL org.opencontainers.image.authors="XenForo Ltd. <support@xenforo.com>"
LABEL org.opencontainers.image.url="https://xenforo.com"
LABEL org.opencontainers.image.source="https://github.com/xenforo-ltd/cli"
LABEL org.opencontainers.image.documentation="https://github.com/xenforo-ltd/cli/blob/main/README.md"
LABEL org.opencontainers.image.licenses="MIT"

ARG PHP_VERSION_IGBINARY
ARG PHP_VERSION_IMAGICK
ARG PHP_VERSION_REDIS
ARG PHP_VERSION_SQLSRV
ARG PHP_VERSION_XDEBUG
ARG PHP_VERSION_YAML

ARG PHP_BUILD_IMAGICK
ARG PHP_BUILD_MYSQL
ARG PHP_BUILD_PGSQL
ARG PHP_BUILD_REDIS
ARG PHP_BUILD_SQLSRV
ARG PHP_INSTALL_FFMPEG

RUN set -eux; \
    docker-php-ext-install -j$(nproc) exif;

RUN set -eux; \
    apk add --no-cache --virtual .php-deps-gd \
        libjpeg-turbo \
        libpng \
        libwebp \
    ; \
    apk add --no-cache --virtual .php-deps-gd-build \
        libjpeg-turbo-dev \
        libpng-dev \
        libwebp-dev \
        zlib-dev \
    ; \
    if [ "$(php-config --vernum)" -ge 80100 ]; then \
        apk add --no-cache --virtual .php-deps-gd-avif \
            aom-libs \
            libavif \
            libdav1d \
        ; \
        apk add --no-cache --virtual .php-deps-gd-build-avif \
            aom-dev \
            dav1d-dev \
            libavif-dev \
        ; \
    fi; \
    if [ "$(php-config --vernum)" -ge 80100 ]; then \
        docker-php-ext-configure gd --with-avif --with-jpeg --with-webp; \
    elif [ "$(php-config --vernum)" -ge 70400 ]; then \
        docker-php-ext-configure gd --with-jpeg --with-webp; \
    else \
        docker-php-ext-configure gd --with-jpeg-dir --with-webp-dir; \
    fi; \
    docker-php-ext-install -j$(nproc) gd; \
    apk del .php-deps-gd-build; \
    if [ "$(php-config --vernum)" -ge 80100 ]; then \
        apk del .php-deps-gd-build-avif; \
    fi;

RUN set -eux; \
    apk add --no-cache --virtual .php-deps-gmp gmp; \
    apk add --no-cache --virtual .php-deps-gmp-build gmp-dev; \
    docker-php-ext-install -j$(nproc) gmp; \
    apk del .php-deps-gmp-build;

RUN set -eux; \
    apk add --no-cache --virtual .php-deps-intl icu-libs; \
    apk add --no-cache --virtual .php-deps-intl-build icu-dev; \
    docker-php-ext-install -j$(nproc) intl; \
    apk del .php-deps-intl-build;

RUN set -eux; \
    if [ "$(php-config --vernum)" -lt 80500 ]; then \
        docker-php-ext-install -j$(nproc) opcache; \
    fi; \
    echo "opcache.jit=tracing" >> "${PHP_INI_DIR}/conf.d/docker-php-ext-opcache.ini"; \
    echo "opcache.jit_buffer_size=128M" >> "${PHP_INI_DIR}/conf.d/docker-php-ext-opcache.ini";

RUN set -eux; \
    # zip and unzip are bundled for XFS \
    apk add --no-cache --virtual .php-deps-zip \
        libzip \
        unzip \
        zip \
    ; \
    apk add --no-cache --virtual .php-deps-zip-build libzip-dev; \
    docker-php-ext-install -j$(nproc) zip; \
    apk del .php-deps-zip-build;

RUN set -eux; \
    if [ "${PHP_INSTALL_FFMPEG}" -ne 0 ]; then \
        apk add --no-cache ffmpeg; \
    fi;

RUN --mount=type=bind,from=ghcr.io/php/pie:bin,source=/pie,target=/usr/local/bin/pie \
    set -eux; \
    export PIE_WORKING_DIRECTORY=/tmp/pie; \
    if [ "${PHP_BUILD_IMAGICK}" -ne 0 ]; then \
        apk add --no-cache --virtual .php-deps-imagick \
            imagemagick \
            libgomp \
        ; \
        apk add --no-cache --virtual .php-deps-imagick-build \
            ${PHPIZE_DEPS} \
            imagemagick-dev \
            libtool \
        ; \
        pie install --no-interaction --no-cache "imagick/imagick:${PHP_VERSION_IMAGICK}"; \
        rm -rf /tmp/pie; \
        apk del .php-deps-imagick-build; \
    fi;

RUN --mount=type=bind,from=ghcr.io/php/pie:bin,source=/pie,target=/usr/local/bin/pie \
    set -eux; \
    export PIE_WORKING_DIRECTORY=/tmp/pie; \
    if [ "${PHP_BUILD_REDIS}" -ne 0 ]; then \
        apk add --no-cache --virtual .php-deps-redis-build ${PHPIZE_DEPS} libtool; \
        pie install --no-interaction --no-cache "igbinary/igbinary:${PHP_VERSION_IGBINARY}"; \
        pie install --no-interaction --no-cache \
            --enable-redis-igbinary \
            "phpredis/phpredis:${PHP_VERSION_REDIS}"; \
        rm -rf /tmp/pie; \
        apk del .php-deps-redis-build; \
    fi;

RUN set -eux; \
    if [ "${PHP_BUILD_MYSQL}" -ne 0 ]; then \
        docker-php-ext-install -j$(nproc) mysqli; \
        docker-php-ext-install -j$(nproc) pdo_mysql; \
    fi;

RUN --mount=type=bind,from=ghcr.io/php/pie:bin,source=/pie,target=/usr/local/bin/pie \
    set -eux; \
    export PIE_WORKING_DIRECTORY=/tmp/pie; \
    if [ "${PHP_BUILD_PGSQL}" -ne 0 ]; then \
        apk add --no-cache --virtual .php-deps-pgsql \
            postgresql-libs \
            yaml \
        ; \
        apk add --no-cache --virtual .php-deps-pgsql-build \
            ${PHPIZE_DEPS} \
            libtool \
            postgresql-dev \
            yaml-dev \
        ; \
        docker-php-ext-install -j$(nproc) pgsql; \
        docker-php-ext-install -j$(nproc) pdo_pgsql; \
        pie install --no-interaction --no-cache "pecl/yaml:${PHP_VERSION_YAML}"; \
        rm -rf /tmp/pie; \
        apk del .php-deps-pgsql-build; \
    fi;

ARG TARGETARCH

RUN --mount=type=bind,from=ghcr.io/php/pie:bin,source=/pie,target=/usr/local/bin/pie \
    set -eux; \
    export PIE_WORKING_DIRECTORY=/tmp/pie; \
    if [ "${PHP_BUILD_SQLSRV}" -ne 0 ]; then \
        odbc_package="msodbcsql18_18.7.1.1-1_${TARGETARCH}.apk"; \
        curl -fsSLO "https://download.microsoft.com/download/ade174b7-8cea-4543-91a6-c33ae320c2f0/${odbc_package}"; \
        apk add --no-cache --allow-untrusted "${odbc_package}"; \
        rm "${odbc_package}"; \
        apk add --no-cache --virtual .php-deps-sqlsrv unixodbc; \
        apk add --no-cache --virtual .php-deps-sqlsrv-build \
            ${PHPIZE_DEPS} \
            libtool \
            unixodbc-dev \
        ; \
        pie install --no-interaction --no-cache "microsoft/sqlsrv:${PHP_VERSION_SQLSRV}"; \
        pie install --no-interaction --no-cache "microsoft/pdo_sqlsrv:${PHP_VERSION_SQLSRV}"; \
        rm -rf /tmp/pie; \
        apk del .php-deps-sqlsrv-build; \
    fi;

RUN set -eux; \
    echo "post_max_size=128M" >> "${PHP_INI_DIR}/conf.d/xenforo.ini"; \
    echo "upload_max_filesize=128M" >> "${PHP_INI_DIR}/conf.d/xenforo.ini"; \
    echo "opcache.revalidate_freq=0" >> "${PHP_INI_DIR}/conf.d/docker-php-ext-opcache.ini";

WORKDIR /var/www/html

VOLUME /var/www/html

FROM composer:${PHP_VERSION_COMPOSER} AS composer

FROM php AS development

ARG PHP_VERSION_XDEBUG

RUN --mount=type=bind,from=ghcr.io/php/pie:bin,source=/pie,target=/usr/local/bin/pie \
    set -eux; \
    export PIE_WORKING_DIRECTORY=/tmp/pie; \
    apk add --no-cache --virtual .php-deps-xdebug-build \
        ${PHPIZE_DEPS} \
        libtool \
        linux-headers \
    ; \
    pie install --no-interaction --no-cache "xdebug/xdebug:${PHP_VERSION_XDEBUG}"; \
    rm -rf /tmp/pie; \
    apk del .php-deps-xdebug-build;

COPY --from=composer /usr/bin/composer /usr/local/bin/composer

RUN set -eux; \
    cp "${PHP_INI_DIR}/php.ini-development" "${PHP_INI_DIR}/php.ini";

FROM php AS production

RUN set -eux; \
    cp "${PHP_INI_DIR}/php.ini-production" "${PHP_INI_DIR}/php.ini"; \
    echo "opcache.validate_timestamps=0" >> "${PHP_INI_DIR}/conf.d/docker-php-ext-opcache.ini";

FROM production AS ci

COPY --from=composer /usr/bin/composer /usr/local/bin/composer

FROM production AS release

COPY --chown=www-data:www-data . .
COPY --chown=www-data:www-data src/config.docker.php src/config.php
