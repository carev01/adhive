// Minimal tests to validate the patch surface
const { detectChallengeSignals } = require('./playwright-scraper.js');

function test(label, html, status, res, expected) {
  const { blocked } = detectChallengeSignals(html, status, res, null);
  console.log(`TEST ${label}:`, blocked ? 'BLOCKED' : 'OK', `(status=${status}, res=${res})` , 'expected=', expected ? 'BLOCKED' : 'OK');
}

// Basic sanity
 test("Just a moment tiny", "<html>Just a moment</html>", 200, 60, true);
 test("No issues", "<html>no issues</html>", 200, 60, false);
 test("Captcha present", "<html>captcha</html>", 200, 40, true);
 test("Cloudflare only", "<html>cloudflare</html>", 200, 60, false);

// Visible challenge simulation
 test("Visible iframe recaptcha", `<html><body><iframe class='g-recaptcha'></iframe></body></html>`, 200, 20, true);
 test("Visible text check", `<html>Just a moment</html>`, 200, 100, true);
 test("Weak signal + large resources guarded", `<html>cloudflare</html>`, 200, 60, false);

// Cross-domain redirect scenario: not captured here as input/html-only
 test("No patch cross-domain", `<html>contents</html>`, 200, 100, false);
