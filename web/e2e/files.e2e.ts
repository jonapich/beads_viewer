import { expect, row, test } from "./fixtures";
import { CAPTURE_DIR } from "./lib/tracker";

// Images inline, clickable local paths and Show in Explorer (beads-c2o.64). The "Captured target" bead
// names files under .beads/captures/e2e/ that exist in the writable tracker's project directory.

const fileURL = (path: string) => `/api/v1/file?path=${encodeURIComponent(path)}`;

test.describe("files in bead text", () => {
  test.beforeEach(async ({ page, rw }) => {
    await rw.signIn(page);
  });

  test("the description shows images as thumbnails, a lightbox steps through them, and nothing remote loads", async ({ page, rw, tracker: { ids } }) => {
    const remote: string[] = [];
    page.on("request", (r) => {
      if (r.url().includes("example.invalid")) remote.push(r.url());
    });
    await page.goto(`${rw.base}/issues/${ids.captured}`);

    const first = page.locator("img[alt=first]");
    await expect(first).toBeVisible();
    await expect(first).toHaveJSProperty("naturalWidth", 10);
    await expect(page.locator("img[alt=second]")).toBeVisible();

    // A bare path and a code span that name an image are links; a missing explicit one is a chip with its reason.
    const description = page.locator(".prose").first();
    await expect(description.getByRole("link", { name: `${CAPTURE_DIR}/one.png` })).toHaveAttribute("href", /\/api\/v1\/file\?path=/);
    await expect(description.locator("code, a.code").filter({ hasText: `${CAPTURE_DIR}/two.png` })).toBeVisible();
    await expect(description.getByText("not found")).toBeVisible();
    await expect(description.getByRole("button", { name: "Copy path" })).toBeVisible();

    // A remote image is a link chip, never loaded.
    const remoteChip = description.getByRole("link", { name: /tracker/ });
    await expect(remoteChip).toHaveAttribute("href", "https://example.invalid/pixel.png");
    await expect(remoteChip).toHaveAttribute("target", "_blank");
    expect(remote).toEqual([]);

    // A plain click opens the lightbox on that image, with Prev/Next buttons and no other shortcuts.
    await first.click();
    const box = page.getByRole("dialog");
    await expect(box).toBeVisible();
    await expect(box).toContainText("one.png");
    await expect(box.getByRole("button", { name: "Previous image" })).toBeDisabled();
    await box.getByRole("button", { name: "Next image" }).click();
    await expect(box).toContainText("2 /");
    await expect(box.getByRole("link", { name: /Open in new tab/ })).toHaveAttribute("href", /\/api\/v1\/file\?path=/);
    await page.keyboard.press("Escape");
    await expect(box).toBeHidden();
    // Focus goes back to the thumbnail that opened it.
    await expect(page.locator("a.thumb").filter({ has: first })).toBeFocused();

    // The backdrop closes it too.
    await first.click();
    await expect(box).toBeVisible();
    await page.mouse.click(5, 5); // outside the dialog box, on the backdrop
    await expect(box).toBeHidden();
  });

  test("a sign-off lists its captures as tiles, with chips for what cannot be shown", async ({ page, rw, tracker: { ids } }) => {
    await page.goto(`${rw.base}/issues/${ids.captured}`);
    const card = page.locator('[data-needs="Sign-off"]');
    const strip = card.locator(".strip");
    await expect(strip.locator("a.thumb.tile")).toHaveCount(2);
    await expect(strip.locator("a.thumb.tile img").first()).toHaveJSProperty("naturalWidth", 10);
    await expect(strip.getByText("one.png")).toBeVisible();
    // A file outside the served directories, and one that does not exist: chips with the reason and Copy path.
    await expect(strip.locator(".chip").filter({ hasText: "win.ini" })).toContainText("outside the project");
    await expect(strip.locator(".chip").filter({ hasText: "missing.png" })).toContainText("not found");
    await expect(strip.getByRole("button", { name: "Copy path" })).toHaveCount(2);
    // where-to-see: the image path and the PDF are links.
    await expect(card.getByRole("link", { name: `${CAPTURE_DIR}/one.png` })).toHaveAttribute("href", /file\?path=/);
    await expect(card.getByRole("link", { name: `${CAPTURE_DIR}/doc.pdf` })).toHaveAttribute("target", "_blank");

    // The tiles are one lightbox group: two images.
    await strip.locator("a.thumb.tile").first().click();
    await expect(page.getByRole("dialog")).toContainText("1 / 2");
    await page.getByRole("dialog").getByRole("button", { name: "Close preview" }).click();
    await expect(page.getByRole("dialog")).toBeHidden();
  });

  test("the agent desk pane and the peek drawer show the captures too", async ({ page, rw, tracker: { ids } }) => {
    await page.goto(`${rw.base}/desk?tab=signoff&id=${ids.captured}`);
    await expect(page.locator(".pane").locator("a.thumb.tile")).toHaveCount(2);

    await page.goto(`${rw.base}/issues`);
    await page.getByRole("button", { name: "Flat" }).click();
    await row(page, ids.captured).locator(".t-btn").click();
    const drawer = page.getByRole("complementary", { name: "Issue details" });
    await expect(drawer.locator("a.thumb.compact")).toHaveCount(2);
    await expect(drawer.getByRole("link", { name: `${CAPTURE_DIR}/doc.pdf` })).toBeVisible();
  });

  test("the file endpoint serves files inside the project only, with the sandbox policy", async ({ page, rw }) => {
    const get = (path: string) => page.request.get(rw.base + fileURL(path));

    const png = await get(`${CAPTURE_DIR}/one.png`);
    expect(png.status()).toBe(200);
    expect(png.headers()["content-type"]).toBe("image/png");
    expect(png.headers()["content-security-policy"]).toContain("sandbox");
    expect(png.headers()["cache-control"]).toBe("private, no-cache");

    expect((await get("../outside.png")).status()).toBe(403);
    expect((await get("C:\\Windows\\win.ini")).status()).toBe(403);
    expect((await get("\\\\host\\share\\a.png")).status()).toBe(400);
    expect((await get(`${CAPTURE_DIR}/missing.png`)).status()).toBe(404);
    expect((await get(".beads/config.yaml")).status()).toBe(415); // exists, but not an allowed type

    // The same checks on the batch endpoint.
    const q = ["one.png", "missing.png"].map((n) => `path=${encodeURIComponent(`${CAPTURE_DIR}/${n}`)}`).join("&");
    const stat = (await (await page.request.get(`${rw.base}/api/v1/file/stat?${q}`)).json()) as { files: Array<{ status: string; kind?: string }> };
    expect(stat.files.map((f) => f.status)).toEqual(["ok", "missing"]);
    expect(stat.files[0]?.kind).toBe("image");
  });

  // The hand check of beads-c2o.64: Edge's PDF viewer renders a PDF served with the sandbox policy.
  test("a PDF served under the sandbox policy renders in the browser's viewer", async ({ page, rw }) => {
    const resp = await page.goto(rw.base + fileURL(`${CAPTURE_DIR}/doc.pdf`));
    expect(resp?.status()).toBe(200);
    expect(resp?.headers()["content-type"]).toBe("application/pdf");
    await expect(page.locator('embed[type="application/pdf"]')).toHaveCount(1);
  });

  test("Show in Explorer posts the resolved path and never opens the file", async ({ page, rw, tracker: { ids } }) => {
    test.skip(process.platform !== "win32", "Explorer is Windows only");
    // The test must not start a real Explorer window: the request is answered here.
    const posted: Array<{ method: string; body: unknown }> = [];
    await page.route("**/api/v1/file/reveal", async (route) => {
      posted.push({ method: route.request().method(), body: route.request().postDataJSON() });
      await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ path: "x" }) });
    });
    const meta = (await (await page.request.get(rw.base + "/api/v1/meta")).json()) as { capabilities: { files: boolean; reveal: boolean } };
    expect(meta.capabilities).toMatchObject({ files: true, reveal: true });

    await page.goto(`${rw.base}/issues/${ids.captured}`);
    const strip = page.locator('[data-needs="Sign-off"] .strip');
    await strip.getByRole("button", { name: "Show in Explorer" }).first().click();
    await expect.poll(() => posted.length).toBe(1);
    expect(posted[0]?.method).toBe("POST");
    expect((posted[0]?.body as { path: string }).path).toMatch(/one\.png$/);

    // From the lightbox too.
    await strip.locator("a.thumb.tile").first().click();
    await page.getByRole("dialog").getByRole("button", { name: "Show in Explorer" }).click();
    await expect.poll(() => posted.length).toBe(2);
  });

  test("a failed reveal says why", async ({ page, rw, gate, tracker: { ids } }) => {
    test.skip(process.platform !== "win32", "Explorer is Windows only");
    gate.allow(/Failed to load resource/);
    await page.route("**/api/v1/file/reveal", (route) =>
      route.fulfill({ status: 403, contentType: "application/json", body: JSON.stringify({ error: { code: "outside_roots", message: "that path is outside the directories this server may serve" } }) }),
    );
    // The failing response logs a console error for the resource.
    await page.goto(`${rw.base}/issues/${ids.captured}`);
    await page.locator('[data-needs="Sign-off"] .strip').getByRole("button", { name: "Show in Explorer" }).first().click();
    await expect(page.getByRole("log", { name: "Notifications" })).toContainText("outside the directories");
  });
});

test.describe("files on a read-only tracker", () => {
  test("files are still served but nothing can be revealed", async ({ page, ro }) => {
    await ro.signIn(page);
    const meta = (await (await page.request.get(ro.base + "/api/v1/meta")).json()) as { capabilities: { files: boolean; reveal: boolean; writes: boolean } };
    expect(meta.capabilities).toMatchObject({ files: true, writes: false, reveal: false });
    const res = await page.request.post(ro.base + "/api/v1/file/reveal", {
      data: { path: `${CAPTURE_DIR}/one.png` },
      headers: { Origin: ro.base },
    });
    expect([403, 501]).toContain(res.status());
  });
});
