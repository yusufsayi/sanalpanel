<?php
/**
 * phpMyAdmin signon endpoint
 * Panel kısa-ömürlü token üretir, kullanıcı /pma-signon.php?t=<token> URL'ine yönlendirilir,
 * bu script token'i panel API'sine sorar (X-Internal-Auth header ile), DB credentials döner,
 * session'a yazar ve phpMyAdmin'e yönlendirir.
 */
declare(strict_types=1);
session_name('pma_signon');
ini_set('session.cookie_path', '/');
session_start();

if (empty($_GET['t'])) {
    http_response_code(400);
    die('Token eksik. Panel uzerinden gecis yapin.');
}
$token = (string)$_GET['t'];
if (!preg_match('/^[a-f0-9]{16,128}$/', $token)) {
    http_response_code(400);
    die('Token formati gecersiz.');
}

$internalToken = trim((string)@file_get_contents('/etc/girginospanel/pma-internal.token'));
if ($internalToken === '') {
    http_response_code(500);
    die('PMA internal token sunucuda yok.');
}

$ch = curl_init('http://127.0.0.1:8080/api/v1/internal/pma-redeem');
curl_setopt_array($ch, [
    CURLOPT_RETURNTRANSFER => true,
    CURLOPT_POST           => true,
    CURLOPT_POSTFIELDS     => json_encode(['token' => $token]),
    CURLOPT_HTTPHEADER     => [
        'Content-Type: application/json',
        'X-Internal-Auth: ' . $internalToken,
    ],
    CURLOPT_CONNECTTIMEOUT => 3,
    CURLOPT_TIMEOUT        => 5,
]);
$resp = curl_exec($ch);
$code = curl_getinfo($ch, CURLINFO_HTTP_CODE);
curl_close($ch);

if ($code !== 200 || !$resp) {
    http_response_code(401);
    die('Token bozulamadi (kod ' . (int)$code . '). Panel uzerinden yeniden deneyin.');
}
$data = json_decode($resp, true);
if (!is_array($data) || empty($data['kullanici'])) {
    http_response_code(500);
    die('Sunucudan beklenmedik yanit.');
}

$_SESSION['PMA_single_signon_user']     = $data['kullanici'];
$_SESSION['PMA_single_signon_password'] = $data['parola'];
$_SESSION['PMA_single_signon_host'] = 'localhost';
$_SESSION['PMA_single_signon_only_db']  = [$data['db']];
session_write_close();

header('Location: /pma/', true, 302);
exit;
