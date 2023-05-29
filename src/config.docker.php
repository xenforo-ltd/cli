<?php

if (!function_exists('getenv_docker'))
{
	function getenv_docker(
		string $name,
		?string $cast = null,
		string $default = ''
	)
	{
		$filename = getenv("{$name}_FILE");
		if ($filename !== false)
		{
			$value = file_get_contents($filename);
		}
		else
		{
			$value = getenv($name);
		}

		if ($value === false)
		{
			return $default;
		}

		$value = trim($value);

		if ($cast !== null)
		{
			settype($value, $cast);
		}

		return $value;
	}
}

$config['db']['host'] = getenv_docker('XF_DB_HOST');
$config['db']['port'] = getenv_docker('XF_DB_PORT', 'int');
$config['db']['username'] = getenv_docker('XF_DB_USER');
$config['db']['password'] = getenv_docker('XF_DB_PASSWORD');
$config['db']['dbname'] = getenv_docker('XF_DB_DATABASE');
$config['db']['socket'] = null;

$config['fullUnicode'] = true;

$config['cache']['enabled'] = getenv_docker('XF_ENABLE_CACHE', 'bool');
$config['cache']['sessions'] = getenv_docker('XF_CACHE_SESSIONS', 'bool');
$config['cache']['provider'] = 'Redis';
$config['cache']['config'] = [
	'host' => getenv_docker('XF_CACHE_HOST'),
];

if (getenv_docker('XF_ENABLE_CACHE') && getenv_docker('XF_CACHE_PAGES'))
{
	$config['pageCache']['enabled'] = true;
	$config['cache']['context']['page']['provider'] = 'Redis';
	$config['cache']['context']['page']['config'] = [
		'host' => getenv_docker('XF_CACHE_HOST'),
	];
}

$config['debug'] = getenv_docker('XF_DEBUG', 'bool');

if (getenv_docker('XF_DEVELOPMENT'))
{
	$config['development']['enabled'] = true;
	$config['development']['skipAddOns'] = [];
	$config['development']['throwJobErrors'] = true;
	$config['development']['fullJs'] = true;

	$config['enableLivePayments'] = false;

	$c->extend('options', function ($options)
	{
		/** @var \ArrayObject $options */
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

	$c['session.admin'] = function (\XF\Container $c): \XF\Session\Session
	{
		$session = new \XF\Session\Session($c['session.admin.storage'], [
			'cookie' => 'session_admin',
			'lifetime' => 86400,
		]);
		return $session->start($c['request']);
	};
}

$config['adminColorHueShift'] = getenv_docker('XF_ADMIN_HUE_SHIFT', 'int');
$config['cookie']['prefix'] = getenv_docker('XF_COOKIE_PREFIX');
$config['enableAddOnArchiveInstaller'] = true;
$config['enableMail'] = getenv_docker('XF_ENABLE_MAIL', 'bool');

$c->extend('options', function ($options)
{
	/** @var \ArrayObject $options */
	$options['boardTitle'] = getenv_docker('XF_TITLE');

	$options['defaultEmailAddress'] = getenv_docker('XF_EMAIL');
	$options['contactEmailAddress'] = getenv_docker('XF_CONTACT_EMAIL');

	$options['useFriendlyUrls'] = true;

	if (getenv_docker('XF_ENABLE_MAIL'))
	{
		$options['emailTransport'] = [
			'emailTransport' => 'smtp',
			'smtpHost' => getenv_docker('XF_MAIL_HOST'),
			'smtpPort' => getenv_docker('XF_MAIL_PORT', 'int'),
			'smtpAuth' => 'login',
			'smtpLoginUsername' => getenv_docker('XF_MAIL_USERNAME'),
			'smtpLoginPassword' => getenv_docker('XF_MAIL_PASSWORD'),
			'smtpEncrypt' => getenv_docker('XF_MAIL_ENCRYPT'),
		];
	}

	$options['xfesConfig']['host'] = getenv_docker('XF_XFES_SEARCH_HOST');

	if (getenv_docker('XF_ENABLE_IMAGICK'))
	{
		$options['imageLibrary'] = 'imPecl';
	}

	if (getenv_docker('XF_ENABLE_XFMG_FFMPEG'))
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
