package main

import (
	"reflect"
	"testing"
)

// One media, one second-path-parameter and one JSON operation; the 404 and the non-id or non-integer fields must stay out of the columns, and the response ids at any depth take their x-vv-entity kind.
const operationsFixture = `
paths:
  /api/videos/{id}/thumbnail:
    get:
      responses:
        "200":
          content:
            image/jpeg:
              schema:
                type: string
        "404":
          $ref: "#/components/responses/NotFound"
  /api/videos/{id}/tags/{tagId}/confirm:
    post:
      parameters:
        - $ref: "#/components/parameters/VideoId"
        - name: tagId
          in: path
      responses:
        "204":
          description: "Confirmed"
  /api/video-edits:
    post:
      requestBody:
        content:
          application/json:
            schema:
              $ref: "#/components/schemas/VideoEditRequest"
      responses:
        "200":
          $ref: "#/components/responses/Edited"
components:
  parameters:
    VideoId:
      name: id
      in: path
  responses:
    NotFound:
      content:
        application/json:
          schema:
            $ref: "#/components/schemas/Error"
    Edited:
      content:
        application/json:
          schema:
            $ref: "#/components/schemas/VideoEditResponse"
  schemas:
    Error:
      type: object
    VideoEditResponse:
      type: object
      properties:
        items:
          type: array
          items:
            $ref: "#/components/schemas/EditedVideo"
        undoActivityId:
          type: integer
          x-vv-entity: activity
    EditedVideo:
      x-vv-entity: video
      type: object
      properties:
        id:
          type: integer
        tagIds:
          type: array
          items:
            type: integer
            x-vv-entity: tag
        performer:
          $ref: "#/components/schemas/CreditRef"
          x-vv-entity: person
        next:
          $ref: "#/components/schemas/EditedVideo"
    CreditRef:
      type: object
      properties:
        id:
          type: integer
        name:
          type: string
    PersonId:
      type: integer
    VideoEditRequest:
      type: object
      properties:
        videoIds:
          type: array
          items:
            type: integer
        personId:
          $ref: "#/components/schemas/PersonId"
        parentId:
          type: [integer, "null"]
        playbackId:
          type: string
        externalIds:
          type: array
          items:
            type: object
        title:
          type: string
`

func TestOperationTableDerivesColumnsFromTheSchema(t *testing.T) {
	ops, err := operationTable([]byte(operationsFixture))
	if err != nil {
		t.Fatal(err)
	}
	want := []operation{
		{
			Method: "POST", Path: "/api/video-edits", Response: "VideoEditResponse", Request: "VideoEditRequest",
			BodyIDs: []string{"parentId", "personId", "videoIds"},
			ResponseIDs: []idField{
				{Field: "/undoActivityId", Kind: "activity"},
				{Field: "/items/*/id", Kind: "video"},
				{Field: "/items/*/tagIds/*", Kind: "tag"},
				{Field: "/items/*/performer/id", Kind: "person"},
			},
		},
		{Method: "POST", Path: "/api/videos/{id}/tags/{tagId}/confirm", PathParams: []string{"tagId"}},
		{Method: "GET", Path: "/api/videos/{id}/thumbnail", Streams: true},
	}
	if !reflect.DeepEqual(ops, want) {
		t.Errorf("表の列が openapi.yaml と合わない\n got: %+v\nwant: %+v", ops, want)
	}
}
