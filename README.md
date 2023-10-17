# Runik API

`v0.1.0`
User management API built with PostgreSQL, Redis, Fiber, and GORM

## Authorization

### Global Auth

This is a master token that can be sent for some endpoints
It can be set by the `GLOBAL_AUTH` environment variable

### Session Auth

Use a session token for authenticating as a user

## Endpoints

Base endpoint: /api/v1/

### POST /users

[Global Auth](#global-auth)

Create user

| Field    | Constraints             | Description                       |
| :------- | :---------------------- | :-------------------------------- |
| email    | required, email         | user email                        |
| password | required, min 8, max 32 | user password                     |
| url      | required, url           | url to send in verification email |

Returns

| Field | Type      | Description                 |
| :---- | :-------- | :-------------------------- |
| id    | snowflake | User ID in snowflake format |

### GET /users

Get all users

Response

[User](#user)[]

### POST /users/sessions

[Global Auth](#global-auth)
Create a session

| Field    | Constraints              | Description                               |
| :------- | :----------------------- | :---------------------------------------- |
| email    | required, email          | login email                               |
| password | required, min 8, max 32  | login password                            |
| expire   | required, boolean        | whether session will expire after 10 days |
| ip       | default to client ip, ip | ip that created session                   |

### DELETE /users/sessions/:token

[Session Auth](#session-auth)

Response

| Field   | Type    | Description                       |
| :------ | :------ | :-------------------------------- |
| success | boolean | whether any sessions were deleted |

### GET /users/me

[Session Auth](#session-auth)

Get the signed in user

Response
[User](#user)

### PUT /users/me/email

[Session Auth](#session-auth)

Update the signed in users email and issue a verification request

| Field | Constraints     | Description                       |
| :---- | :-------------- | :-------------------------------- |
| email | required, email | new email                         |
| url   | required, url   | url to send in verification email |

### PUT /users/me/password

[Session Auth](#session-auth)

Update the signed in users email and issue a verification request

| Field       | Constraints                       | Description               |
| :---------- | :-------------------------------- | :------------------------ |
| oldPassword | required, min 8, max 32           | the current password      |
| newPassword | required, required, min 8, max 32 | the password to update to |

### POST /users/verify

[Global Auth](#global-auth)
Create a verification request

| Field | Constraints     | Description                       |
| :---- | :-------------- | :-------------------------------- |
| email | required, email | new email                         |
| url   | required, url   | url to send in verification email |

### PUT /users/verify/:token

Confirm a verification request

### POST /users/reset

[Global Auth](#global-auth)

Create a password reset request

| Field | Constraints     | Description                |
| :---- | :-------------- | :------------------------- |
| email | required, email | new email                  |
| url   | required, url   | url to send in reset email |

### PUT /users/reset/:token

Update a password from a reset request

## Types

| Field    | Constraints             | Description      |
| :------- | :---------------------- | :--------------- |
| password | required, min 8, max 32 | the new password |

### User

| Field    | Type         | Description                 |
| :------- | :----------- | :-------------------------- |
| id       | Snowflake    | ID of user                  |
| email    | email/string | email of user               |
| verified | boolean      | whether `email` is verified |
