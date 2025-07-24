<?php

use XF\Container;
use XF\Options;
use XF\Session\Session;

if (!function_exists('getenv_docker'))
{
	function getenv_docker(string $name, string $default = ''): string
	{
		$filename = getenv("{$name}_FILE");
		if ($filename !== false)
		{
			return rtrim(file_get_contents($filename), "\r\n");
		}

		$value = getenv($name);
		if ($value === false)
		{
			return $default;
		}

		return $value;
	}
}

if (getenv_docker('XF_DB_REPLICATION'))
{
	$config['db']['adapterClass'] = 'XF\Db\Pdo\MysqlReplicationAdapter';

	$config['db']['read']['host'] = getenv_docker('XF_DB_HOST_READ', 'localhost');
	$config['db']['read']['port'] = (int) getenv_docker('XF_DB_PORT', '3306');
	$config['db']['read']['username'] = getenv_docker('XF_DB_USER');
	$config['db']['read']['password'] = getenv_docker('XF_DB_PASSWORD');
	$config['db']['read']['dbname'] = getenv_docker('XF_DB_DATABASE');

	$config['db']['write']['host'] = getenv_docker('XF_DB_HOST', 'localhost');
	$config['db']['write']['port'] = (int) getenv_docker('XF_DB_PORT', '3306');
	$config['db']['write']['username'] = getenv_docker('XF_DB_USER');
	$config['db']['write']['password'] = getenv_docker('XF_DB_PASSWORD');
	$config['db']['write']['dbname'] = getenv_docker('XF_DB_DATABASE');
}
else
{
	$config['db']['host'] = getenv_docker('XF_DB_HOST', 'localhost');
	$config['db']['port'] = (int) getenv_docker('XF_DB_PORT', '3306');
	$config['db']['username'] = getenv_docker('XF_DB_USER');
	$config['db']['password'] = getenv_docker('XF_DB_PASSWORD');
	$config['db']['dbname'] = getenv_docker('XF_DB_DATABASE');
}

$config['fullUnicode'] = true;
$config['searchInnoDb'] = true;

$config['enableCssSplitting'] = true;
$config['enableContentLength'] = false;
$config['enableGzip'] = false;

$config['cache']['enabled'] = (bool) getenv_docker('XF_CACHE_ENABLE');
$config['cache']['sessions'] = (bool) getenv_docker('XF_CACHE_SESSIONS');
$config['cache']['provider'] = 'Redis';
$config['cache']['config'] = [
	'host' => getenv_docker('XF_CACHE_HOST'),
];

if (getenv_docker('XF_CACHE_ENABLE') && getenv_docker('XF_CACHE_PAGES'))
{
	$config['pageCache']['enabled'] = true;
	$config['cache']['context']['page']['provider'] = 'Redis';
	$config['cache']['context']['page']['config'] = [
		'host' => getenv_docker('XF_CACHE_HOST'),
	];
}

$config['debug'] = (bool) getenv_docker('XF_DEBUG');

if (getenv_docker('XF_DEVELOPMENT'))
{
	$config['development']['enabled'] = true;
	$config['development']['skipAddOns'] = [];
	$config['development']['throwJobErrors'] = true;
	$config['development']['fullJs'] = true;

	$config['enableLivePayments'] = false;

	$c->extend('options', function ($options)
	{
		/** @var Options $options */
		$options['collectServerStats'] = [
			'configured' => 1,
			'enabled' => 0,
		];

		$options['captcha'] = '';
		$options['registrationSetup']['emailConfirmation'] = 0;
		$options['registrationTimer'] = 0;
		$options['sitemapAutoSubmit']['enabled'] = 0;

		return $options;
	});

	$c['session.admin'] = function (Container $c): Session
	{
		$session = new Session($c['session.admin.storage'], [
			'cookie' => 'session_admin',
			'lifetime' => 86400,
		]);

		return $session->start($c['request']);
	};
}

$config['adminColorHueShift'] = (int) getenv_docker('XF_ADMIN_HUE_SHIFT');
$config['cookie']['prefix'] = getenv_docker('XF_COOKIE_PREFIX', 'xf_');
$config['enableAddOnArchiveInstaller'] = true;
$config['enableMail'] = (bool) getenv_docker('XF_MAIL_ENABLE');

$c->extend('options', function ($options)
{
	/** @var Options $options */
	$options['boardTitle'] = getenv_docker('XF_TITLE', 'XenForo');

	$options['defaultEmailAddress'] = getenv_docker('XF_EMAIL');
	$options['contactEmailAddress'] = getenv_docker('XF_CONTACT_EMAIL');

	$options['useFriendlyUrls'] = true;

	if (getenv_docker('XF_MAIL_ENABLE'))
	{
		$options['emailTransport'] = [
			'emailTransport' => 'smtp',
			'smtpHost' => getenv_docker('XF_MAIL_HOST'),
			'smtpPort' => (int) getenv_docker('XF_MAIL_PORT'),
			'smtpAuth' => 'login',
			'smtpLoginUsername' => getenv_docker('XF_MAIL_USERNAME'),
			'smtpLoginPassword' => getenv_docker('XF_MAIL_PASSWORD'),
			'smtpSsl' => (bool) getenv_docker('XF_MAIL_SSL'),
		];
	}

	if (isset($options['xfesConfig']))
	{
		$options['xfesConfig']['host'] = getenv_docker('XF_XFES_SEARCH_HOST', 'localhost');
		$options['xfesConfig']['port'] = (int) getenv_docker('XF_XFES_SEARCH_PORT', '9200');
		$options['xfesConfig']['username'] = getenv_docker('XF_XFES_SEARCH_USER');
		$options['xfesConfig']['password'] = getenv_docker('XF_XFES_SEARCH_PASSWORD');
		$options['xfesConfig']['index'] = getenv_docker('XF_XFES_SEARCH_INDEX');
	}

	if (getenv_docker('XF_IMAGICK_ENABLE'))
	{
		$options['imageLibrary'] = 'imPecl';
	}

	if (getenv_docker('XF_XFMG_FFMPEG_ENABLE'))
	{
		$options['xfmgFfmpeg'] = [
			'enabled' => '1',
			'ffmpegPath' => '/usr/bin/ffmpeg',
			'thumbnail' => '1',
			'poster' => '1',
			'transcode' => '1',
			'phpPath' => '/usr/local/bin/php',
			'limit' => '4',
			'forceTranscode' => '1',
		];
	}

	return $options;
});

$override = __DIR__ . '/config.override.php';
if (file_exists($override) && is_file($override))
{
	require $override;
}
