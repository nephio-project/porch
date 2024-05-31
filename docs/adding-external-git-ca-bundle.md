# Adding an external Git CaBundle

To enable the porch server to communicate with a custom git deployment over HTTPS, we must:
1. Provide a additional args flag `use-git-cabundle=true` to the porch-server deployment.
2. Provide an additional kubernetes secret containing the relevant certificate chain in the form of a cabundle.

The secret itself must meet the following criteria:

- exist in the same `namespace` as the Repository CR (Custom Resource) that requires it
- be named specifically `<namespace>-ca-bundle`
- have a Data key named `ca.crt` containing the relevant ca certificate (chain)

For example, a Git Repository is hosted over HTTPS at the following URL:

`https://my-gitlab.com/joe.bloggs/blueprints.git`

Before creating the new Repository in the **gitlab** namespace, we must create a secret that fulfils the criteria above.

`kubectl create secret generic gitlab-ca-bundle --namespace=gitlab --from-file=ca.crt`

Which would produce the following:

```
apiVersion: v1
kind: Secret
metadata:
  name: gitlab-ca-bundle
  namespace: gitlab
type: Opaque
data:
  ca.crt: LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUNuakNDQWdHZ0F3SUJBZ0lRTEdmUytUK3YyRDZDczh1MVBlUUlKREFLQmdncWhrak9QUVFEQkRBZE1Sc3cKR1FZRFZRUURFeEpqWlhKMExXMWhibUZuWlhJdWJHOWpZV3d3SGhjTk1qUXdOVE14TVRFeU5qTXlXaGNOTWpRdwpPREk1TVRFeU5qTXlXakFWTVJNd0VRWURWUVFGRXdveE1qTTBOVFkzT0Rrd01JSUJJakFOQmdrcWhraUc5dzBCCkFRRUZBQU9DQVE4QU1JSUJDZ0tDQVFFQXhCUUtWMEVzQ1JOOGxuV3lQR1ZWNXJwam5QZkI2emszK0N4cEp2NVMKUWhpMG1KbDI0elV1WWZjRzNxdFUva1NuREdjK3NQRUY0RmlOcUlsSTByWHBQSXBPazhKbjEvZU1VT3RkZUUyNgpSWEZBWktjeDVvdUJyZVNja3hsN2RPVkJnOE1EM1h5RU1PQU5nM0hJZ1J4ZWx2U2p1dy8vMURhSlRnK0lBS0dUCkgrOVlRVFcrZDIwSk5wQlR3NkdnQlRsYmdqL2FMRWEwOXVYSVBjK0JUSkpXRThIeDhkVjFNbEtHRFlDU29qZFgKbG9TN1FIa0dsSVk3M0NPZVVGWEVnTlFVVmZaZHdreXNsT3F4WmdXUTNZTFZHcEFyRitjOVdyUGpQQU5NQWtORQpPdHRvaG8zTlRxQ3FST3JEa0RMYWdsU1BKSUd1K25TcU5veVVxSUlWWkV5R1dRSURBUUFCbzJBd1hqQU9CZ05WCkhROEJBZjhFQkFNQ0JhQXdEQVlEVlIwVEFRSC9CQUl3QURBZkJnTlZIU01FR0RBV2dCUitFZTVDTnVJSkcwZjkKV3J3VzdqYUZFeVdzb1RBZEJnTlZIUkVFRmpBVWdoSm5hWFJzWVdJdVpYaGhiWEJzWlM1amIyMHdDZ1lJS29aSQp6ajBFQXdRRGdZb0FNSUdHQWtGLzRyNUM4bnkwdGVIMVJlRzdDdXJHYk02SzMzdTFDZ29GTkthajIva2ovYzlhCnZwODY0eFJKM2ZVSXZGMEtzL1dNUHNad2w2bjMxUWtXT2VpM01aYWtBUUpCREw0Kyt4UUxkMS9uVWdqOW1zN2MKUUx3NXVEMGxqU0xrUS9mOTJGYy91WHc4QWVDck5XcVRqcDEycDJ6MkUzOXRyWWc1a2UvY2VTaWFPUm16eUJuTwpTUTg9Ci0tLS0tRU5EIENFUlRJRklDQVRFLS0tLS0=
```