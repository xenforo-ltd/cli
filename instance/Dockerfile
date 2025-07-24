# syntax=docker/dockerfile:1

ARG PHP_VERSION=8.4
ARG PHP_VARIANT=fpm

ARG PHP_VERSION_COMPOSER=2.8
ARG PHP_VERSION_IGBINARY=3.2.16
ARG PHP_VERSION_IMAGICK=3.8.0
ARG PHP_VERSION_REDIS=6.2.0
ARG PHP_VERSION_XDEBUG=3.4.5
ARG PHP_VERSION_YAML=2.2.4

ARG PHP_BUILD_IMAGICK=0
ARG PHP_BUILD_MYSQL=0
ARG PHP_BUILD_PGSQL=0
ARG PHP_BUILD_REDIS=0
ARG PHP_BUILD_SQLSRV=0
ARG PHP_INSTALL_FFMPEG=0

FROM php:${PHP_VERSION}-${PHP_VARIANT}-alpine AS php

ARG PHP_VERSION_IGBINARY
ARG PHP_VERSION_IMAGICK
ARG PHP_VERSION_REDIS
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
    apk del .php-deps-gd-build;\
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
    docker-php-ext-install -j$(nproc) opcache; \
    echo "opcache.jit=tracing" >> "${PHP_INI_DIR}/conf.d/docker-php-ext-opcache.ini"; \
    echo "opcache.jit_buffer_size=128M" >> "${PHP_INI_DIR}/conf.d/docker-php-ext-opcache.ini";

RUN set -eux; \
    apk add --no-cache --virtual .php-deps-zip libzip; \
    apk add --no-cache --virtual .php-deps-zip-build libzip-dev; \
    docker-php-ext-install -j$(nproc) zip; \
    apk del .php-deps-zip-build;

RUN set -eux; \
    if [ "${PHP_INSTALL_FFMPEG}" -ne 0 ]; then \
        apk add --no-cache ffmpeg; \
    fi;

RUN set -eux; \
    if [ "${PHP_BUILD_IMAGICK}" -ne 0 ]; then \
        apk add --no-cache --virtual .php-deps-imagick \
            imagemagick \
            libgomp \
        ; \
        apk add --no-cache --virtual .php-deps-imagick-build \
            ${PHPIZE_DEPS} \
            imagemagick-dev \
        ; \
        pecl install imagick-${PHP_VERSION_IMAGICK}; \
        docker-php-ext-enable imagick; \
        rm -rf /tmp/pear; \
        apk del .php-deps-imagick-build; \
    fi;

RUN set -eux; \
    if [ "${PHP_BUILD_REDIS}" -ne 0 ]; then \
        apk add --no-cache --virtual .php-deps-redis-build ${PHPIZE_DEPS}; \
        pecl install igbinary-${PHP_VERSION_IGBINARY}; \
        pecl install \
            --configureoptions 'enable-redis-igbinary="yes"' \
            redis-${PHP_VERSION_REDIS}; \
        docker-php-ext-enable igbinary; \
        docker-php-ext-enable redis; \
        rm -rf /tmp/pear; \
        apk del .php-deps-redis-build; \
    fi;

RUN set -eux; \
    if [ "${PHP_BUILD_MYSQL}" -ne 0 ]; then \
        docker-php-ext-install -j$(nproc) mysqli; \
        docker-php-ext-install -j$(nproc) pdo_mysql; \
    fi;

RUN set -eux; \
    if [ "${PHP_BUILD_PGSQL}" -ne 0 ]; then \
        apk add --no-cache --virtual .php-deps-pgsql \
            postgresql-libs \
            yaml \
        ; \
        apk add --no-cache --virtual .php-deps-pgsql-build \
            ${PHPIZE_DEPS} \
            postgresql-dev \
            yaml-dev \
        ; \
        docker-php-ext-install -j$(nproc) pgsql; \
        docker-php-ext-install -j$(nproc) pdo_pgsql; \
        pecl install yaml-${PHP_VERSION_YAML}; \
        docker-php-ext-enable yaml; \
        rm -rf /tmp/pear; \
        apk del .php-deps-pgsql-build; \
    fi;

RUN set -eux; \
    if [ "${PHP_BUILD_SQLSRV}" -ne 0 ]; then \
        curl -O https://download.microsoft.com/download/fae28b9a-d880-42fd-9b98-d779f0fdd77f/msodbcsql18_18.5.1.1-1_arm64.apk; \
        apk add --allow-untrusted msodbcsql18_18.5.1.1-1_arm64.apk; \
        rm msodbcsql18_18.5.1.1-1_arm64.apk; \
        apk add --no-cache --virtual .php-deps-sqlsrv unixodbc; \
        apk add --no-cache --virtual .php-deps-sqlsrv-build \
            ${PHPIZE_DEPS} \
            unixodbc-dev \
        ; \
        pecl install sqlsrv; \
        pecl install pdo_sqlsrv; \
        docker-php-ext-enable sqlsrv; \
        docker-php-ext-enable pdo_sqlsrv; \
        rm -rf /tmp/pear; \
        apk del .php-deps-sqlsrv-build; \
    fi;

RUN set -eux; \
    echo "post_max_size=128M" >> "${PHP_INI_DIR}/conf.d/xenforo.ini"; \
    echo "upload_max_filesize=128M" >> "${PHP_INI_DIR}/conf.d/xenforo.ini";

VOLUME /var/www/html

FROM composer:${PHP_VERSION_COMPOSER} AS composer

FROM php AS development

ARG PHP_VERSION_XDEBUG

RUN set -eux; \
    apk add --no-cache --virtual .php-deps-xdebug-build \
        ${PHPIZE_DEPS} \
        linux-headers \
    ; \
    pecl install xdebug-${PHP_VERSION_XDEBUG}; \
    docker-php-ext-enable xdebug; \
    rm -rf /tmp/pear; \
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
