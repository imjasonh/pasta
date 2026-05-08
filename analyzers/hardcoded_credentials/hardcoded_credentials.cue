// hardcoded_credentials flags string literals that look like API keys
// or secrets — AWS access keys, GitHub tokens, generic 32+ byte
// hex/base64 blobs assigned to suspicious-looking identifiers.
//
// Cross-language: every grammar pasta supports has a string-literal
// node type, and the patterns are syntactic. We list the common ones
// in the `node` union so the rule applies broadly.

package hardcoded_credentials

import "pasta.dev/schema"

hardcoded_credentials: schema.#Analyzer & {
	name:    "hardcoded_credentials"
	version: "0.1.0"
	doc:     "Flag string literals that look like API keys or secrets"
	facts: {}

	rules: {
		// AWS access keys: 20 chars, AKIA / ASIA prefix, all caps + digits.
		aws_access_key: {
			name: "aws_access_key"
			doc:  "String looks like an AWS access key (AKIA*/ASIA* prefix)"
			languages: ["*"]
			requires: []
			provides: []
			match: {
				node: ["string", "string_literal", "interpreted_string_literal", "raw_string_literal", "string_fragment"]
				where: [{op: "matches", args: ["@_root", "(AKIA|ASIA)[A-Z0-9]{16}"]}]
			}
			diagnose: {
				message:  "string literal looks like an AWS access key — secrets do not belong in source"
				severity: "error"
			}
		}

		// GitHub fine-grained / classic tokens.
		github_token: {
			name: "github_token"
			doc:  "String looks like a GitHub token (ghp_/gho_/ghs_/ghu_/ghr_/github_pat_)"
			languages: ["*"]
			requires: []
			provides: []
			match: {
				node: ["string", "string_literal", "interpreted_string_literal", "raw_string_literal", "string_fragment"]
				where: [{op: "matches", args: ["@_root", "(ghp_|gho_|ghs_|ghu_|ghr_|github_pat_)[A-Za-z0-9_]{20,}"]}]
			}
			diagnose: {
				message:  "string literal looks like a GitHub token"
				severity: "error"
			}
		}

		// Slack tokens.
		slack_token: {
			name: "slack_token"
			doc:  "String looks like a Slack token (xox[abprsoa]-...)"
			languages: ["*"]
			requires: []
			provides: []
			match: {
				node: ["string", "string_literal", "interpreted_string_literal", "raw_string_literal", "string_fragment"]
				where: [{op: "matches", args: ["@_root", "xox[abprsoa]-[A-Za-z0-9-]{10,}"]}]
			}
			diagnose: {
				message:  "string literal looks like a Slack token"
				severity: "error"
			}
		}

		// Generic private-key markers (PEM headers).
		pem_private_key: {
			name: "pem_private_key"
			doc:  "String contains a PEM-encoded private key marker"
			languages: ["*"]
			requires: []
			provides: []
			match: {
				node: ["string", "string_literal", "interpreted_string_literal", "raw_string_literal", "string_fragment"]
				where: [{op: "matches", args: ["@_root", "BEGIN (RSA |EC |DSA |OPENSSH |PGP )?PRIVATE KEY"]}]
			}
			diagnose: {
				message:  "string contains a PEM private-key header"
				severity: "error"
			}
		}
	}
}
