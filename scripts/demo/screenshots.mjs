// Takes screenshots of the running dApp with a test wallet connected to a local Anvil chain.
// Usage: node scripts/demo/screenshots.mjs <output-dir>
// Expects the app on http://127.0.0.1:3000, Anvil on :8545 and the indexer on :8080.
import { chromium } from "playwright";
import { mkdirSync } from "node:fs";

const OUT = process.argv[2] ?? "screenshots";
const APP = "http://127.0.0.1:3000";
// Anvil test account #1 (unlocked on Anvil, so the "wallet" can send transactions without signing).
const ACCOUNT = "0x70997970C51812dc3A010C7d01b50e0d17dc79C8";

mkdirSync(OUT, { recursive: true });

// A minimal EIP-1193 wallet that forwards to Anvil. Injected before the app loads.
const injectWallet = ([account]) => {
  let id = 0;
  const rpc = async (method, params) => {
    const res = await fetch("http://127.0.0.1:8545", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ jsonrpc: "2.0", id: ++id, method, params: params ?? [] }),
    });
    const json = await res.json();
    if (json.error) {
      const err = new Error(json.error.message);
      err.code = json.error.code;
      err.data = json.error.data;
      throw err;
    }
    return json.result;
  };
  const provider = {
    isMetaMask: true,
    async request({ method, params }) {
      switch (method) {
        case "eth_requestAccounts":
        case "eth_accounts":
          return [account];
        case "eth_chainId":
          return "0x7a69";
        case "net_version":
          return "31337";
        case "wallet_requestPermissions":
        case "wallet_getPermissions":
          return [{ parentCapability: "eth_accounts" }];
        case "wallet_switchEthereumChain":
        case "wallet_addEthereumChain":
        case "wallet_revokePermissions":
          return null;
        default:
          return rpc(method, params);
      }
    },
    on() {
      return provider;
    },
    removeListener() {
      return provider;
    },
  };
  window.ethereum = provider;
};

async function newPage(browser, viewport) {
  const context = await browser.newContext({ viewport, deviceScaleFactor: 2, colorScheme: "dark" });
  await context.addInitScript(injectWallet, [ACCOUNT]);
  const page = await context.newPage();
  page.on("pageerror", (e) => console.log("page error:", e.message));
  return page;
}

async function connect(page) {
  const positions = page.getByText("Your positions");
  const connectButton = page.getByRole("button", { name: "Connect Wallet" }).first();
  // The injected wallet may auto-connect on load; only open the modal if it didn't.
  await Promise.race([positions.waitFor(), connectButton.waitFor()]);
  if (await positions.isVisible()) {
    await page.locator("table.table tbody tr").first().waitFor();
    await page.waitForTimeout(2500);
    return;
  }
  await connectButton.click();
  const dialog = page.getByRole("dialog");
  await dialog.waitFor();
  // The wallet list re-renders while it animates in, so retry the click until we're connected.
  for (let attempt = 1; attempt <= 8; attempt++) {
    await page.waitForTimeout(1000);
    if (await page.getByText("Your positions").isVisible()) break;
    const wallet = dialog.getByRole("button", { name: /MetaMask/ }).first();
    try {
      await wallet.click({ force: true, timeout: 3000 });
    } catch (e) {
      console.log(`connect attempt ${attempt}: ${e.message.split("\n")[0]}`);
    }
  }
  await page.getByText("Your positions").waitFor();
  await page.locator("table.table tbody tr").first().waitFor();
  await page.waitForTimeout(2500); // let reads and the indexer panels settle
}

async function waitForTx(page) {
  await page.locator(".toast.success, .toast.error").waitFor({ timeout: 60_000 });
  if (await page.locator(".toast.error").count()) {
    throw new Error(`Transaction failed: ${await page.locator(".toast").innerText()}`);
  }
}

const browser = await chromium.launch();
let current;
try {
  // 1. Landing page, wallet not connected.
  const visitor = await newPage(browser, { width: 1280, height: 900 });
  await visitor.goto(APP);
  await visitor.getByText("Total staked").first().waitFor();
  await visitor.waitForTimeout(3000);
  await visitor.screenshot({ path: `${OUT}/01-landing.png`, fullPage: true });
  await visitor.context().close();

  // 2. Connected dashboard.
  const page = await newPage(browser, { width: 1280, height: 900 });
  current = page;
  await page.goto(APP);
  await connect(page);
  await page.screenshot({ path: `${OUT}/02-dashboard.png`, fullPage: true });

  // 3. Stake form with an amount and a 90-day lock selected.
  await page.getByLabel("Amount to stake").fill("750");
  await page.getByRole("radio", { name: /90 days/ }).click();
  await page.waitForTimeout(500);
  await page.locator(".grid .card").first().screenshot({ path: `${OUT}/03-stake-form.png` });

  // 4. Approve, then stake, and capture the confirmation.
  await page.getByRole("button", { name: "Approve STK" }).click();
  await waitForTx(page);
  await page.getByRole("button", { name: /Stake with 2x boost/ }).click();
  await waitForTx(page);
  await page.waitForTimeout(2000);
  await page.screenshot({ path: `${OUT}/04-after-stake.png` });

  // 5. Rewards and positions close-up.
  await page.locator(".grid .stack").first().screenshot({ path: `${OUT}/05-rewards-and-positions.png` });

  // 6. Indexer panels: chart, leaderboard, activity.
  await page.getByText("Recent activity").scrollIntoViewIfNeeded();
  await page.waitForTimeout(3000);
  const panels = page.locator(".grid").nth(1);
  await panels.screenshot({ path: `${OUT}/06-activity-leaderboard.png` });
  await page.context().close();

  // 7. Mobile.
  const mobile = await newPage(browser, { width: 390, height: 844 });
  current = mobile;
  await mobile.goto(APP);
  await connect(mobile);
  await mobile.screenshot({ path: `${OUT}/07-mobile.png`, fullPage: true });
  await mobile.context().close();

  console.log("Screenshots saved to", OUT);
} catch (e) {
  if (current) {
    await current.screenshot({ path: `${OUT}/debug-failure.png` }).catch(() => {});
    const dialog = await current.getByRole("dialog").innerText().catch(() => "(no dialog)");
    console.log("Dialog text on failure:\n", dialog);
  }
  throw e;
} finally {
  await browser.close();
}
