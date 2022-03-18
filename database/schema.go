package database

import (
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var (
	// Schema defines a mapping between a collection name and the indexes that
	// must exist for that collection.
	Schema = map[string][]mongo.IndexModel{
		collUsers: {
			{
				Keys:    bson.D{{"sub", 1}},
				Options: options.Index().SetName("sub_unique").SetUnique(true),
			},
		},
		collSkylinks: {
			{
				Keys:    bson.D{{"skylink", 1}},
				Options: options.Index().SetName("skylink_unique").SetUnique(true),
			},
		},
		collUploads: {
			{
				Keys:    bson.D{{"user_id", 1}},
				Options: options.Index().SetName("user_id"),
			},
			{
				Keys:    bson.D{{"skylink_id", 1}},
				Options: options.Index().SetName("skylink_id"),
			},
		},
		collDownloads: {
			{
				Keys:    bson.D{{"user_id", 1}},
				Options: options.Index().SetName("user_id"),
			},
			{
				Keys:    bson.D{{"skylink_id", 1}},
				Options: options.Index().SetName("skylink_id"),
			},
		},
		collRegistryReads: {
			{
				Keys:    bson.D{{"user_id", 1}},
				Options: options.Index().SetName("user_id"),
			},
		},
		collRegistryWrites: {
			{
				Keys:    bson.D{{"user_id", 1}},
				Options: options.Index().SetName("user_id"),
			},
		},
		collEmails: {
			{
				Keys:    bson.D{{"failed_attempts", 1}},
				Options: options.Index().SetName("failed_attempts"),
			},
			{
				Keys:    bson.D{{"locked_by", 1}},
				Options: options.Index().SetName("locked_by"),
			},
			{
				Keys:    bson.D{{"sent_at", 1}},
				Options: options.Index().SetName("sent_at"),
			},
			{
				Keys:    bson.D{{"sent_by", 1}},
				Options: options.Index().SetName("sent_by"),
			},
		},
		collChallenges: {
			{
				Keys:    bson.D{{"challenge", 1}},
				Options: options.Index().SetName("challenge"),
			},
			{
				Keys:    bson.D{{"type", 1}},
				Options: options.Index().SetName("type"),
			},
			{
				Keys:    bson.D{{"expires_at", 1}},
				Options: options.Index().SetName("expires_at"),
			},
		},
		collUnconfirmedUserUpdates: {
			{
				Keys:    bson.D{{"challenge_id", 1}},
				Options: options.Index().SetName("challenge_id"),
			},
			{
				Keys:    bson.D{{"expires_at", 1}},
				Options: options.Index().SetName("expires_at"),
			},
		},
		collConfiguration: {
			{
				Keys:    bson.D{{"key", 1}},
				Options: options.Index().SetName("key_unique").SetUnique(true),
			},
		},
		collAPIKeys: {
			{
				Keys:    bson.D{{"key", 1}},
				Options: options.Index().SetName("key_unique").SetUnique(true),
			},
			{
				Keys:    bson.D{{"user_id", 1}},
				Options: options.Index().SetName("user_id"),
			},
		},
	}
)
