using System.Text.Json.Serialization;

namespace ClaudeUsageWidget.Services;

internal sealed record CursorCredentialRequest(
    [property: JsonPropertyName("cookie")] string? Cookie,
    [property: JsonPropertyName("access_token")] string? AccessToken);

internal sealed record CursorCredentialResponse(
    [property: JsonPropertyName("provider")] string? Provider,
    [property: JsonPropertyName("refetched")] bool? Refetched,
    [property: JsonPropertyName("usage")] UsageWire? Usage);

internal sealed record UsageWire(
    [property: JsonPropertyName("provider_name")] string? ProviderName,
    [property: JsonPropertyName("primary_label")] string? PrimaryLabel,
    [property: JsonPropertyName("secondary_label")] string? SecondaryLabel,
    [property: JsonPropertyName("show_secondary")] bool? ShowSecondary,
    [property: JsonPropertyName("subtitle")] string? Subtitle,
    [property: JsonPropertyName("primary_status_text")] string? PrimaryStatusText,
    [property: JsonPropertyName("secondary_status_text")] string? SecondaryStatusText,
    [property: JsonPropertyName("reauth_command")] string? ReauthCommand,
    [property: JsonPropertyName("current")] UsageBucketWire? Current,
    [property: JsonPropertyName("weekly")] UsageBucketWire? Weekly,
    [property: JsonPropertyName("buckets")] IReadOnlyList<UsageBucketDetailWire?>? Buckets,
    [property: JsonPropertyName("error")] string? Error,
    [property: JsonPropertyName("needs_reauth")] bool? NeedsReauth,
    [property: JsonPropertyName("is_success")] bool? IsSuccess);

internal sealed record UsageBucketWire(
    [property: JsonPropertyName("utilization")] double? Utilization,
    [property: JsonPropertyName("resets_at")] DateTime? ResetsAt);

internal sealed record UsageBucketDetailWire(
    [property: JsonPropertyName("id")] string? Id,
    [property: JsonPropertyName("label")] string? Label,
    [property: JsonPropertyName("utilization")] double? Utilization,
    [property: JsonPropertyName("resets_at")] DateTime? ResetsAt,
    [property: JsonPropertyName("status_text")] string? StatusText);

internal sealed record HealthWire(
    [property: JsonPropertyName("status")] string? Status,
    [property: JsonPropertyName("version")] string? Version);

internal sealed class WireValidationException : Exception;
