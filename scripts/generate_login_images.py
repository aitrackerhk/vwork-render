"""
Generate login page right-panel branding images for V-sys products using Gemini.
Uses Gemini 2.0 Flash with image generation capability.
Output: 3 PNG images saved to vwork/web/static/
"""

import os
import sys
from google import genai
from google.genai import types

# API Key
API_KEY = "AIzaSyCHTCHR_Mod0J1zZSj5MHxKAZtIJXHlEC4"
OUTPUT_DIR = os.path.join(os.path.dirname(__file__), "..", "web", "static")

# Product-specific prompts - all share a similar dark, premium style
# but each has unique visual elements reflecting the product's purpose
# IMPORTANT: All prompts emphasize NO TEXT whatsoever
NO_TEXT_RULE = (
    "CRITICAL: The image must contain ABSOLUTELY NO TEXT, NO LETTERS, NO NUMBERS, "
    "NO WORDS, NO TYPOGRAPHY, NO CHARACTERS, NO SYMBOLS, NO LOGOS, NO WATERMARKS. "
    "This is purely abstract art with zero text elements. "
    "Do NOT include any writing, labels, or readable characters of any kind. "
)

PRODUCTS = {
    "vwork": {
        "filename": "login_bg_vwork.png",
        "prompt": (
            "A stunning dark abstract wallpaper, portrait orientation 900x1200. "
            "Deep navy black (#0a0f1a) background. "
            "Elegant soft-glow mesh gradient orbs floating in space — large diffused spheres "
            "of deep indigo (#4338ca) and rich violet (#7c3aed), blending into the dark background "
            "with beautiful bokeh-like light diffusion. "
            "Very subtle thin geometric wireframe lines connecting in a constellation pattern, "
            "barely visible, adding depth. "
            "A few tiny scattered particles of light like distant stars. "
            "The composition feels like a premium tech company's hero image — think Apple, Stripe, or Linear dark mode. "
            "Ultra clean, atmospheric, moody. Beautiful negative space. "
            "Photographic quality, 8K render feel. "
            + NO_TEXT_RULE
        ),
    },
    "voffice": {
        "filename": "login_bg_voffice.png",
        "prompt": (
            "A stunning dark abstract wallpaper, portrait orientation 900x1200. "
            "Deep dark charcoal-black (#0a0a0f) background. "
            "Smooth flowing aurora-like ribbons of light sweeping diagonally across the image — "
            "colors transition from deep red (#dc2626) through burnt orange (#ea580c) to warm amber (#f59e0b), "
            "with beautiful gradient blending into the dark background. "
            "The light ribbons are soft and ethereal, like silk fabric floating in darkness. "
            "Subtle glass-morphism translucent rectangular shapes layered in the background, "
            "barely visible, suggesting document layers. "
            "The composition feels like a premium SaaS product's landing page — think Notion or Figma dark mode. "
            "Ultra clean, warm and bold yet sophisticated. Beautiful depth and atmosphere. "
            "Photographic quality, 8K render feel. "
            + NO_TEXT_RULE
        ),
    },
    "vmarket": {
        "filename": "login_bg_vmarket.png",
        "prompt": (
            "A stunning dark abstract wallpaper, portrait orientation 900x1200. "
            "Deep dark purple-black (#0c0716) background. "
            "Beautiful abstract light formations — warm glowing orbs and curved light trails "
            "in rich amber (#f59e0b), soft coral (#fb7185), and deep rose (#e11d48), "
            "creating an inviting warm atmosphere against the cold dark background. "
            "The lights feel like abstract representations of a vibrant nighttime cityscape or marketplace. "
            "Subtle lens flare effects and light dispersion creating rainbow-edge highlights. "
            "Scattered micro-particles of warm light like floating embers. "
            "The composition feels like a premium lifestyle brand's visual — think luxury e-commerce dark mode. "
            "Ultra clean, warm yet sophisticated, inviting. Beautiful contrast between warm lights and dark space. "
            "Photographic quality, 8K render feel. "
            + NO_TEXT_RULE
        ),
    },
}


def generate_image(client, product_key, product_config):
    """Generate a single product background image."""
    print(f"\n{'='*50}")
    print(f"Generating image for: {product_key}")
    print(f"Output: {product_config['filename']}")
    print(f"{'='*50}")

    try:
        response = client.models.generate_content(
            model="gemini-2.0-flash-exp-image-generation",
            contents=product_config["prompt"],
            config=types.GenerateContentConfig(
                response_modalities=["IMAGE", "TEXT"],
            ),
        )

        # Extract and save image
        if response.candidates:
            for part in response.candidates[0].content.parts:
                if part.inline_data is not None:
                    image_data = part.inline_data.data
                    output_path = os.path.join(OUTPUT_DIR, product_config["filename"])
                    with open(output_path, "wb") as f:
                        f.write(image_data)
                    print(f"[OK] Saved: {output_path} ({len(image_data)} bytes)")
                    return True
                elif part.text is not None:
                    print(f"[INFO] Text response: {part.text[:200]}")

        print(f"[ERROR] No image generated for {product_key}")
        return False

    except Exception as e:
        print(f"[ERROR] Failed to generate {product_key}: {e}")
        return False


def main():
    print("V-sys Login Page Image Generator")
    print(f"Output directory: {OUTPUT_DIR}")
    print()

    # Initialize client
    client = genai.Client(api_key=API_KEY)

    results = {}
    for product_key, product_config in PRODUCTS.items():
        success = generate_image(client, product_key, product_config)
        results[product_key] = success

    # Summary
    print(f"\n{'='*50}")
    print("SUMMARY")
    print(f"{'='*50}")
    for product_key, success in results.items():
        status = "OK" if success else "FAILED"
        print(f"  {product_key}: [{status}]")

    if all(results.values()):
        print("\nAll images generated successfully!")
        print("Files saved to web/static/:")
        for config in PRODUCTS.values():
            print(f"  - {config['filename']}")
    else:
        print("\nSome images failed to generate.")
        sys.exit(1)


if __name__ == "__main__":
    main()
