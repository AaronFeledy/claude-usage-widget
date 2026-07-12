using System.Net.Http.Headers;

internal sealed class QueueHandler : HttpMessageHandler
{
    private readonly Queue<object> _responses = new();

    public QueueHandler(params object[] responses)
    {
        foreach (var response in responses) _responses.Enqueue(response);
    }

    public List<HttpRequestMessage> Requests { get; } = new();

    protected override Task<HttpResponseMessage> SendAsync(HttpRequestMessage request, CancellationToken cancellationToken)
    {
        Requests.Add(CloneRequest(request));
        var next = _responses.Dequeue();
        return next switch
        {
            HttpResponseMessage response => Task.FromResult(response),
            Exception exception => Task.FromException<HttpResponseMessage>(exception),
            _ => throw new InvalidOperationException("Unsupported fake response.")
        };
    }

    private static HttpRequestMessage CloneRequest(HttpRequestMessage request)
    {
        var clone = new HttpRequestMessage(request.Method, request.RequestUri);
        foreach (var header in request.Headers) clone.Headers.TryAddWithoutValidation(header.Key, header.Value);
        if (request.Content != null)
        {
            var body = request.Content.ReadAsStringAsync().GetAwaiter().GetResult();
            clone.Content = new StringContent(body);
            foreach (var header in request.Content.Headers) clone.Content.Headers.TryAddWithoutValidation(header.Key, header.Value);
            if (request.Content.Headers.ContentType != null) clone.Content.Headers.ContentType = MediaTypeHeaderValue.Parse(request.Content.Headers.ContentType.ToString());
        }
        return clone;
    }
}
