# syntax=docker/dockerfile:1

ARG PHP_VARIANT=fpm
ARG PHP_VERSION=8.2
ARG PHP_VERSION_COMPOSER=2.8
ARG PHP_VERSION_IGBINARY=3.2.14
ARG PHP_VERSION_IMAGICK=3.7.0
ARG PHP_VERSION_REDIS=5.3.7
ARG PHP_VERSION_XDEBUG=3.2.1
ARG PHP_VERSION_YAML=2.2.3

ARG PHP_BUILD_IMAGICK=0
ARG PHP_BUILD_PGSQL=0
ARG PHP_BUILD_REDIS=0
ARG PHP_INSTALL_FFMPEG=0

FROM php:${PHP_VERSION}-${PHP_VARIANT}-alpine AS php

ARG PHP_VERSION_IGBINARY
ARG PHP_VERSION_IMAGICK
ARG PHP_VERSION_REDIS
ARG PHP_VERSION_XDEBUG
ARG PHP_VERSION_YAML

ARG PHP_BUILD_IMAGICK
ARG PHP_BUILD_PGSQL
ARG PHP_BUILD_REDIS
ARG PHP_INSTALL_FFMPEG

RUN set -eux; \
    docker-php-ext-install -j$(nproc) exif;

RUN set -eux; \
    apk add --no-cache --virtual .php-deps-gd libjpeg-turbo libpng libwebp; \
    apk add --no-cache --virtual .php-deps-gd-build \
        libjpeg-turbo-dev \
        libpng-dev \
        libwebp-dev \
        zlib-dev \
        ; \
    if [ "$(php-config --vernum)" -lt 70400 ]; then \
        docker-php-ext-configure gd --with-jpeg-dir --with-webp-dir; \
    else \
        docker-php-ext-configure gd --with-jpeg --with-webp; \
    fi; \
    docker-php-ext-install -j$(nproc) gd; \
    apk del .php-deps-gd-build;

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
    docker-php-ext-install -j$(nproc) opcache;

RUN set -eux; \
    docker-php-ext-install -j$(nproc) mysqli;

RUN set -eux; \
    apk add --no-cache --virtual .php-deps-zip libzip; \
    apk add --no-cache --virtual .php-deps-zip-build libzip-dev; \
    docker-php-ext-install -j$(nproc) zip; \
    apk del .php-deps-zip-build;

RUN set -eux; \
    if [ "$PHP_INSTALL_FFMPEG" -ne 0 ]; then \
        apk add --no-cache ffmpeg \
        ; \
    fi;

RUN set -eux; \
    if [ "$PHP_BUILD_IMAGICK" -ne 0 ]; then \
        apk add --no-cache --virtual .php-deps-imagick imagemagick libgomp; \
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
    if [ "$PHP_BUILD_REDIS" -ne 0 ]; then \
        apk add --no-cache --virtual .php-deps-redis-build ${PHPIZE_DEPS}; \
        pecl install igbinary-${PHP_VERSION_IGBINARY}; \
        docker-php-ext-enable igbinary; \
        pecl install \
            --configureoptions 'enable-redis-igbinary="yes"' \
            redis-${PHP_VERSION_REDIS}; \
        docker-php-ext-enable redis; \
        rm -rf /tmp/pear; \
        apk del .php-deps-redis-build; \
    fi;

RUN set -eux; \
    if [ "$PHP_BUILD_PGSQL" -ne 0 ]; then \
        apk add --no-cache --virtual .php-deps-pgsql postgresql-libs yaml; \
        apk add --no-cache --virtual .php-deps-pgsql-build \
            ${PHPIZE_DEPS} \
            pgsql-dev \
            yaml-dev \
            ; \
        docker-php-ext-install -j$(nproc) pgsql; \
        pecl install yaml-${PHP_VERSION_YAML}; \
        docker-php-ext-enable yaml; \
        rm -rf /tmp/pear; \
        apk del .php-deps-pgsql-build; \
    fi;

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
    cp "$PHP_INI_DIR/php.ini-development" "$PHP_INI_DIR/php.ini";

FROM php AS production

RUN set -eux; \
    cp "$PHP_INI_DIR/php.ini-production" "$PHP_INI_DIR/php.ini";

FROM production AS ci

COPY --from=composer /usr/bin/composer /usr/local/bin/composer

FROM production AS release

COPY --chown=www-data:www-data . .
COPY --chown=www-data:www-data src/config.docker.php src/config.php
