# SSE Application Service

This service allows streaming events from the message bus to a web
browser, using [Server-Sent Events](https://developer.mozilla.org/en-US/docs/Web/API/Server-sent_events)
in HTML 5. 

This service serves two HTTP ports - one using the app service SDK with
app service endpoints, and one directly with only one endpoint, for the
event stream. The app service SDK requests can time out, but SSE GET
requests should not time out.

**TODO**: rest of README

